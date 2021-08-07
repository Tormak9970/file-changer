package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Tormak9970/file-changer/logger"
	"github.com/Tormak9970/file-changer/reader"
	"github.com/Tormak9970/file-changer/reader/hash"
	"github.com/Tormak9970/file-changer/reader/tor"
)

//* Build Command: go build -o fileChanger.exe main.go

func zlipDecompress(buff []byte) ([]byte, error) {
	b := bytes.NewReader(buff)
	r, err := zlib.NewReader(b)

	if err != nil {
		fmt.Print(err)
		return nil, err
	}
	var out bytes.Buffer
	io.Copy(&out, r)

	return out.Bytes(), nil
}
func zlipCompress(buff []byte, output string, filename string, cmd string) ([]byte, error) {
	var out []byte
	tmp1 := output + filename + ".tmp"

	err := os.MkdirAll(output, os.ModePerm)
	if err != nil {
		log.Panicln(err)
	}
	err2 := os.WriteFile(tmp1, buff, os.ModePerm)
	if err2 != nil {
		log.Panicln(err2)
	}
	tmp2, err3 := exec.Command(cmd, tmp1).Output()
	if err3 != nil {
		log.Panicln(err3)
	}
	var err4 error
	out, err4 = ioutil.ReadFile(string(tmp2))
	if err4 != nil {
		log.Panicln(err4)
	}
	err5 := os.Remove(string(tmp2))
	if err5 != nil {
		log.Panicln(err5)
	}

	return out, nil
}

func writeFile(data []byte, dir string, outputDir string) {
	if dir == "" {
		return
	}
	path := outputDir + dir

	destination, err := os.Create(path)
	logger.Check(err)

	destination.Write(data)
	destination.Close()
}
func fCopy(src, dst string) (int64, error) {
	_, err := os.Stat(dst)
	if err != nil {
		if os.IsNotExist(err) {
			sourceFileStat, err := os.Stat(src)
			if err != nil {
				return 0, err
			}

			if !sourceFileStat.Mode().IsRegular() {
				return 0, fmt.Errorf("%s is not a regular file", src)
			}

			source, err := os.Open(src)
			if err != nil {
				return 0, err
			}
			defer source.Close()

			destination, err := os.Create(dst)
			if err != nil {
				return 0, err
			}

			defer destination.Close()
			nBytes, err := io.Copy(destination, source)
			return nBytes, err
		}
	}
	return 0, err
}

func fileNameToHash(name string) hash.FileId {
	return hash.FromFilePath(name, 0)
}

func readGOMString(reader reader.SWTORReader, offset uint64) string {
	var strBuff []byte
	oldOffset, _ := reader.Seek(0, 1)
	reader.Seek(int64(offset), 0)
	for true {
		tempBuff := make([]byte, 1)
		_, err := reader.File.Read(tempBuff)
		if err != nil {
			log.Panicln(err)
		}
		curChar := tempBuff[0]

		if curChar == 0 {
			break
		} else {
			strBuff = append(strBuff, curChar)
		}
	}
	reader.Seek(oldOffset, 0)
	return string(strBuff)
}

type BackupObj struct {
	Backup bool   `json:"backup"`
	Path   string `json:"path"`
}

type FileChange struct {
	Hash         []string `json:"hash"`
	Data         CData    `json:"data"`
	IsCompressed bool     `json:"isCompressed"`
}

type NodeChange struct {
	Name         string `json:"name"`
	Data         CData  `json:"data"`
	IsCompressed bool   `json:"isCompressed"`
}

type CData struct {
	File string `json:"file"`
	Zip  string `json:"zip,omitempty"`
}

type Changes struct {
	Files []FileChange `json:"files"`
	Nodes []NodeChange `json:"nodes"`
}

func checkFileMatch(changes []FileChange, hashes hash.HashData) (FileChange, bool) {
	for _, change := range changes {
		if change.Hash[0] == hashes.PH && change.Hash[1] == hashes.SH {
			return change, true
		}
	}
	return FileChange{}, false
}
func checkNodeMatches(changes []NodeChange, gomName string) (NodeChange, bool) {
	for _, change := range changes {
		if change.Name == gomName {
			return change, true
		}
	}
	return NodeChange{}, false
}

func main() {
	var torFiles []string
	var backupObj BackupObj
	var fileChanges Changes

	hashPath := ""
	zipFilePath := ""
	comprCmd := ""
	if len(os.Args) >= 5 {
		err := json.Unmarshal([]byte(os.Args[1]), &torFiles)
		if err != nil {
			fmt.Println(err)
		}

		hashPath = os.Args[2]
		err2 := json.Unmarshal([]byte(os.Args[3]), &backupObj)
		if err2 != nil {
			fmt.Println(err2)
		}
		err3 := json.Unmarshal([]byte(os.Args[4]), &fileChanges)
		if err3 != nil {
			fmt.Println(err3)
		}
		comprCmd = os.Args[5]
		if len(os.Args) > 6 {
			zipFilePath = os.Args[6]
		} else {
			zipFilePath = ""
		}
	}
	if len(torFiles) == 0 || hashPath == "" || backupObj.Path == "" || (len(fileChanges.Files) == 0 && len(fileChanges.Nodes) == 0) || comprCmd == "" {
		return
	}

	hashes := hash.Read(hashPath)
	lastIdxSub := backupObj.Path[0:strings.LastIndex(backupObj.Path, "\\")]
	tmpIdxSub := lastIdxSub[0:strings.LastIndex(lastIdxSub, "\\")+1] + "tmp"

	var zipReader reader.InMemoryZip
	if zipFilePath != "" {
		zipReader = reader.ReadZip(zipFilePath)
	}
	data, nodeData := tor.ReadAll(torFiles)

	filesNoHash := 0

	filesAttempted := 0
	start := time.Now()

	numNodeChanges := len(fileChanges.Nodes)
	numFileChanges := len(fileChanges.Files)
	numChanges := numFileChanges + numNodeChanges
	numNodesSuccessful := 0
	numFilesSuccessful := 0
	numSuccessful := 0

	for _, data := range data {
		if strings.Contains(data.Name, "main_global_1.tor") && numNodeChanges != 0 {
			if backupObj.Backup {
				fCopy(data.Name, backupObj.Path+data.Name[strings.LastIndex(data.Name, "\\"):])
			}
			f, err := os.OpenFile(data.Name, os.O_RDWR, os.ModePerm)
			if err != nil {
				log.Panicln(err)
			}
			defer f.Close()
			swReader := reader.SWTORReader{File: f}

			for i := 0; i < 500; i++ {
				fileName := "/resources/systemgenerated/buckets/" + strconv.Itoa(i) + ".bkt"
				litHashes := hash.FromFilePath(fileName, 0)
				key := strconv.Itoa(int(litHashes.PH)) + "|" + strconv.Itoa(int(litHashes.SH))

				if nData, ok := nodeData[key]; ok {
					filesAttempted++

					oldPos, _ := swReader.Seek(int64(nData.Offset), 0)
					dblbOffset := nData.Offset + uint64(nData.HeaderSize) + 24

					swReader.Seek(int64(dblbOffset), 0)
					dblbSize := swReader.ReadUInt32()
					swReader.ReadUInt32() //dblb header
					swReader.ReadUInt32() //dblb version

					endOffset := nData.Offset + uint64(nData.HeaderSize) + 28 + uint64(dblbSize)

					var j int

					for pos, _ := swReader.Seek(0, 1); pos < int64(endOffset); j++ {
						nodeOffset, _ := swReader.Seek(0, 1)
						fmt.Println(nodeOffset)
						nodeSize := swReader.ReadUInt32()
						if nodeSize == 0 {
							break
						}
						swReader.ReadUInt32()
						swReader.ReadUInt32() //idLo
						swReader.ReadUInt32() //idHi

						swReader.ReadUInt16() //type
						dataOffset := swReader.ReadUInt16()

						nameOffset := swReader.ReadUInt16()
						gomName := readGOMString(swReader, uint64(nodeOffset)+uint64(nameOffset))

						swReader.ReadUInt16()

						swReader.ReadUInt32() //baseClassLo
						swReader.ReadUInt32() //baseClassHi

						swReader.ReadUInt64()

						swReader.ReadUInt16() //uncompressedSize

						swReader.ReadUInt16()
						swReader.ReadUInt16()

						swReader.ReadUInt16() //uncompressedOffset
						if nChng, fndC := checkNodeMatches(fileChanges.Nodes, gomName); fndC {
							fmt.Println("Found Node!")
							numSuccessful++
							numNodesSuccessful++

							//replace node here
							var zipEntr reader.ZipEntry
							if nChng.IsCompressed {
								zipEntr, _ = zipReader.ParseZipNode(nChng.Data.File)
								var err5 error
								zipEntr.Data, err5 = zlipCompress(zipEntr.Data, tmpIdxSub, nChng.Data.File[strings.LastIndex(nChng.Data.File, "\\")+1:], comprCmd)
								if err5 != nil {
									log.Panicln(err5)
								}
							} else {
								uncomprFile, err4 := os.Open(nChng.Data.File)
								if err4 != nil {
									log.Panicln(err4)
								}
								uncomprStat, _ := uncomprFile.Stat()
								uncomprSize := uncomprStat.Size()
								uncomprData := make([]byte, uncomprSize)
								uncomprFile.Read(uncomprData)
								uncomprFile.Close()

								compressed, err5 := zlipCompress(uncomprData, tmpIdxSub, nChng.Data.File[strings.LastIndex(nChng.Data.File, "\\")+1:], comprCmd)
								if err5 != nil {
									log.Panicln(err5)
								}
								zipEntr = reader.ZipEntry{Name: gomName, Data: compressed, CompressedSize: int64(len(compressed)), UncompressedSize: uncomprSize}
							}
							if int(zipEntr.CompressedSize) > int(nodeSize)-int(dataOffset) {
								log.Panicln("Custom node is too large. Needs to be the same size or smaller.")
								if numNodesSuccessful == numNodeChanges {
									swReader.Seek(nodeOffset+((int64(nodeSize)+7)&-8), 0)
									break
								} else {
									continue
								}
							}
							newData := zipEntr.Data[0 : nodeSize-uint32(dataOffset)]

							uncomprSizeArr := make([]byte, 2)
							binary.LittleEndian.PutUint16(uncomprSizeArr, uint16(zipEntr.UncompressedSize))

							_, err7 := swReader.WriteAt(uncomprSizeArr, nodeOffset+40)
							if err7 != nil {
								log.Panicln(err7)
							}

							swReader.WriteAt(newData, nodeOffset+int64(dataOffset))

							if numNodesSuccessful == numNodeChanges {
								swReader.Seek(nodeOffset+((int64(nodeSize)+7)&-8), 0)
								break
							}
						}
						swReader.Seek(nodeOffset+((int64(nodeSize)+7)&-8), 0)
					}

					swReader.Seek(oldPos, 0)
					fmt.Println(numSuccessful, numChanges)
					if numNodesSuccessful == numNodeChanges {
						break
					}
				} else {
					filesNoHash++
				}
			}
		} else {
			for _, fileData := range data.FileList {
				if hashData, ok := hashes[fileData.FileID]; ok {
					filesAttempted++
					hashData := hashData
					fileData := fileData
					if numFilesSuccessful < numFileChanges {
						if fChng, fndChng := checkFileMatch(fileChanges.Files, hashData); fndChng {
							if backupObj.Backup {
								fCopy(fileData.TorFile, backupObj.Path+fileData.TorFile[strings.LastIndex(fileData.TorFile, "\\"):])
							}
							log.Println("Found file. File num", numSuccessful+1)
							numSuccessful++
							numFilesSuccessful++

							//replace file data here
							f, _ := os.OpenFile(fileData.TorFile, os.O_RDWR, 0777)
							swReader := reader.SWTORReader{File: f}

							var zipEntr reader.ZipEntry
							hasInserted := false

							if fileData.CompressionMethod == 1 {
								if fChng.IsCompressed {
									zipEntr, _ = zipReader.ParseZipFile(fChng.Data.File)
									var err5 error
									zipEntr.Data, err5 = zlipCompress(zipEntr.Data, tmpIdxSub, fChng.Data.File[strings.LastIndex(fChng.Data.File, "/")+1:], comprCmd)
									if err5 != nil {
										log.Panicln(err5)
									}
								} else {
									uncomprFile, _ := os.Open(fChng.Data.File)
									uncomprStat, _ := uncomprFile.Stat()
									uncomprSize := uncomprStat.Size()
									uncomprData := make([]byte, uncomprSize)
									uncomprFile.Read(uncomprData)
									uncomprFile.Close()
									compressed, err4 := zlipCompress(uncomprData, tmpIdxSub, hashData.Filename[strings.LastIndex(hashData.Filename, "/")+1:], comprCmd)
									if err4 != nil {
										log.Panicln(err4)
									}
									zipEntr = reader.ZipEntry{Name: hashData.Filename, Data: compressed, CompressedSize: int64(len(compressed)), UncompressedSize: uncomprSize}
								}
								if zipEntr.CompressedSize <= int64(fileData.CompressedSize) {
									newData := make([]byte, fileData.CompressedSize-1)
									copy(newData, zipEntr.Data)
									for k := int(zipEntr.CompressedSize - 1); k < len(newData); k++ {
										newData[k] = 0
									}
									swReader.WriteAt(newData, int64(fileData.Offset)+int64(fileData.HeaderSize))
									hasInserted = true
								}
							} else if fileData.CompressionMethod == 0 {
								if fChng.IsCompressed {
									zipEntr, _ = zipReader.ParseZipFile(fChng.Data.File)
								} else {
									uncomprFile, _ := os.Open(fChng.Data.File)
									uncomprStat, _ := uncomprFile.Stat()
									uncomprSize := uncomprStat.Size()
									uncomprData := make([]byte, uncomprSize)
									uncomprFile.Read(uncomprData)
									uncomprFile.Close()
									zipEntr = reader.ZipEntry{Name: hashData.Filename, Data: uncomprData, CompressedSize: 0, UncompressedSize: uncomprSize}
								}
								if zipEntr.UncompressedSize <= int64(fileData.UnCompressedSize) {
									newData := zipEntr.Data
									for k := len(newData); k < int(fileData.UnCompressedSize-1); k++ {
										newData[k] = 0
									}
									swReader.WriteAt(newData, int64(fileData.Offset)+int64(fileData.HeaderSize))
									hasInserted = true
								}
							} else {
								log.Panicln("Expected 0 or 1 but got", fileData.CompressionMethod)
							}

							if !hasInserted {
								if data.LastTableNumFiles+1 >= 1000 {
									newLastOffset, _ := swReader.Seek(0, 2)

									capacity := make([]byte, 32)
									binary.LittleEndian.PutUint32(capacity, uint32(1000))
									swReader.Write(capacity)

									nextOffset := make([]byte, 4)
									binary.LittleEndian.PutUint64(capacity, uint64(0))
									swReader.Write(nextOffset)

									for g := 0; g < 1000; g++ {
										zeros := make([]byte, 34)
										swReader.Write(zeros)
									}

									byteOffset := make([]byte, 8)
									binary.LittleEndian.PutUint64(byteOffset, uint64(newLastOffset))
									swReader.WriteAt(byteOffset, data.LastTableOffset+4)

									data.LastTableNumFiles = -1
									data.LastTableOffset = newLastOffset
								}

								modFileOffset, _ := swReader.Seek(0, 2)

								//append modded file to the end of the archive
								metaData := make([]byte, fileData.HeaderSize)
								swReader.Seek(int64(fileData.Offset), 0)
								swReader.Read(metaData)

								swReader.Seek(modFileOffset, 0)
								swReader.Write(metaData)
								swReader.Write(zipEntr.Data)

								//add file table entry
								data.LastTableNumFiles++
								swReader.Seek(data.LastTableOffset+12+int64(34*data.LastTableNumFiles), 0)

								modFileOffBytes := make([]byte, 8)
								binary.LittleEndian.PutUint64(modFileOffBytes, uint64(modFileOffset))
								swReader.Write(modFileOffBytes)

								metDatSizeBytes := make([]byte, 4)
								binary.LittleEndian.PutUint32(metDatSizeBytes, fileData.HeaderSize)
								swReader.Write(metDatSizeBytes)

								if fileData.CompressionMethod == 1 {
									comprDatSizeBytes := make([]byte, 4)
									binary.LittleEndian.PutUint32(comprDatSizeBytes, uint32(zipEntr.CompressedSize))
									swReader.Write(comprDatSizeBytes)
								} else {
									comprDatSizeBytes := make([]byte, 4)
									binary.LittleEndian.PutUint32(comprDatSizeBytes, uint32(zipEntr.UncompressedSize))
									swReader.Write(comprDatSizeBytes)
								}

								uncomprDatSizeBytes := make([]byte, 4)
								binary.LittleEndian.PutUint32(uncomprDatSizeBytes, uint32(zipEntr.UncompressedSize))
								swReader.Write(uncomprDatSizeBytes)

								hashBytes := make([]byte, 8)
								binary.LittleEndian.PutUint64(hashBytes, fileData.FileID)
								swReader.Write(hashBytes)

								chckSumBytes := make([]byte, 4)
								binary.LittleEndian.PutUint32(chckSumBytes, fileData.Checksum)
								swReader.Write(chckSumBytes)

								compTypeBytes := make([]byte, 2)
								binary.LittleEndian.PutUint16(compTypeBytes, fileData.CompressionMethod)
								swReader.Write(compTypeBytes)
							}
							f.Close()
						}
						if numSuccessful == numChanges {
							break
						}
						fmt.Println(numSuccessful, numChanges)
					}
				} else {
					filesNoHash++
				}
			}
		}
	}

	diff := time.Now().Sub(start)
	log.Println("duration", fmt.Sprintf("%s", diff))
	if numSuccessful == numChanges {
		os.Exit(0)
	} else {
		os.Exit(1)
	}
}
