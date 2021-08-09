package tor

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/Tormak9970/file-changer/logger"
	"github.com/Tormak9970/file-changer/reader"
	"github.com/Tormak9970/file-changer/reader/hash"
	"github.com/gammazero/workerpool"
)

type TorArchiveStruct struct {
	Name              string
	FileList          []TorFile
	Offset            uint64
	NumTables         int
	LastTableOffset   int64
	LastTableNumFiles int
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
type RelivantInfo struct {
	BackupObj          BackupObj
	FileChanges        Changes
	ComprCmd           string
	ZipReader          reader.InMemoryZip
	FilesNoHash        int
	FilesAttempted     int
	NumNodeChanges     int
	NumFileChanges     int
	NumChanges         int
	NumNodesSuccessful int
	NumFilesSuccessful int
	NumSuccessful      int
	TmpIdxSub          string
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

func zlipCompress(buff []byte, output string, filename string, cmd string) ([]byte, error) {
	var out []byte
	tmp1 := output + "\\" + filename + ".tmp"

	err := os.MkdirAll(output, os.ModePerm)
	if err != nil {
		log.Panicln(err)
	}
	err2 := os.WriteFile(tmp1, buff, os.ModePerm)
	if err2 != nil {
		log.Panicln(err2)
	}
	stderr := new(bytes.Buffer)
	command := exec.Command(cmd, tmp1)
	command.Stderr = stderr
	tmp2, err3 := command.Output()
	if err3 != nil {
		log.Println(stderr.String())
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
func inArr(arr []uint64, val uint64) bool {
	for _, v := range arr {
		if v == val {
			return true
		}
	}
	return false
}

var relInfo RelivantInfo

func ReadAll(torNames []string, hashes map[uint64]hash.HashData, nodeHashes map[string]bool, relInf RelivantInfo) RelivantInfo {
	relInfo = relInf
	pool := workerpool.New(runtime.NumCPU())

	for _, torName := range torNames {
		torName := torName

		if strings.Contains(torName, "main_global_1.tor") {
			pool.Submit(func() {
				readNodeTor(torName, nodeHashes)
			})
		} else {
			pool.Submit(func() {
				read(torName, hashes)
			})
		}
	}
	pool.StopWait()

	return relInfo
}

func read(torName string, hashes map[uint64]hash.HashData) {
	if relInfo.NumFilesSuccessful == relInfo.NumFileChanges {
		fmt.Println("exit 1")
		return
	}
	archive := TorArchiveStruct{}
	archive.Name = torName
	f, err := os.OpenFile(torName, os.O_RDWR, 0777)

	defer f.Close()
	logger.Check(err)
	swReader := reader.SWTORReader{File: f}
	magicNumber := swReader.ReadUInt32()

	if magicNumber != 0x50594D {
		fmt.Println("Not MYP File")
	}

	swReader.Seek(12, 0)

	fileTableOffset := swReader.ReadUInt64()
	archive.Offset = fileTableOffset

	namedFiles := 0
	lastFile := 0

	hasBackedUp := false

	var runAfter []TorFile
	var runAfterZipEntrs []reader.ZipEntry

	foundHashes := make([]uint64, 0)

	for fileTableOffset != 0 {
		archive.NumTables++
		swReader.Seek(int64(fileTableOffset), 0)
		numFiles := int32(swReader.ReadUInt32())
		tempTableOffset := swReader.ReadUInt64()
		namedFiles += int(numFiles)
		for i := int32(0); i < numFiles; i++ {
			debugOffset, _ := swReader.Seek(0, 1)
			offset := swReader.ReadUInt64()
			if offset == 0 {
				swReader.Seek(26, 1)
				continue
			}
			fileData := TorFile{}
			fileData.HeaderOffset = debugOffset
			fileData.HeaderSize = swReader.ReadUInt32()
			fileData.Offset = offset
			fileData.CompressedSize = swReader.ReadUInt32()
			fileData.UnCompressedSize = swReader.ReadUInt32()
			current_position, _ := swReader.Seek(0, 1)
			fileData.SecondaryHash = swReader.ReadUInt32()
			fileData.PrimaryHash = swReader.ReadUInt32()
			swReader.Seek(current_position, 0)
			fileData.FileID = swReader.ReadUInt64()
			fileData.Checksum = swReader.ReadUInt32()
			fileData.CompressionMethod = swReader.ReadUInt16()
			fileData.CRC = fileData.Checksum
			fileData.TorFile = torName
			fileData.TableIdx = i
			lastFile = int(i)

			restorePos, _ := swReader.Seek(0, 1)

			if hashData, ok := hashes[fileData.FileID]; ok {
				relInfo.FilesAttempted++
				hashData := hashData
				fileData := fileData
				if fChng, fndChng := checkFileMatch(relInfo.FileChanges.Files, hashData); fndChng && relInfo.NumFilesSuccessful < relInfo.NumFileChanges {
					if inArr(foundHashes, fileData.FileID) {
						zeros := make([]byte, 34)
						swReader.WriteAt(zeros, fileData.HeaderOffset)
						continue
					} else {
						foundHashes = append(foundHashes, fileData.FileID)
					}
					swReader.Seek(0, 0)
					if relInfo.BackupObj.Backup && !hasBackedUp {
						fCopy(fileData.TorFile, relInfo.BackupObj.Path+fileData.TorFile[strings.LastIndex(fileData.TorFile, "\\"):])
						hasBackedUp = true
					}
					log.Println("Found file. File num", relInfo.NumSuccessful)
					relInfo.NumSuccessful++
					relInfo.NumFilesSuccessful++

					//replace file data here

					var zipEntr reader.ZipEntry
					hasInserted := false

					if fileData.CompressionMethod == 1 {
						if fChng.IsCompressed {
							zE, _ := relInfo.ZipReader.ParseZipFile(fChng.Data.File)
							compressed, err5 := zlipCompress(zE.Data, relInfo.TmpIdxSub, hashData.Filename[strings.LastIndex(hashData.Filename, "/")+1:], relInfo.ComprCmd)
							if err5 != nil {
								log.Panicln(err5)
							}
							zipEntr = reader.ZipEntry{Name: hashData.Filename, Data: compressed, CompressedSize: int64(len(compressed)), UncompressedSize: zE.UncompressedSize}
						} else {
							uncomprFile, _ := os.Open(fChng.Data.File)
							uncomprStat, _ := uncomprFile.Stat()
							uncomprSize := uncomprStat.Size()
							uncomprData := make([]byte, uncomprSize)
							uncomprFile.Read(uncomprData)
							uncomprFile.Close()
							compressed, err4 := zlipCompress(uncomprData, relInfo.TmpIdxSub, hashData.Filename[strings.LastIndex(hashData.Filename, "/")+1:], relInfo.ComprCmd)
							if err4 != nil {
								log.Panicln(err4)
							}
							zipEntr = reader.ZipEntry{Name: hashData.Filename, Data: compressed, CompressedSize: int64(len(compressed)), UncompressedSize: uncomprSize}
						}
						if zipEntr.CompressedSize <= int64(fileData.CompressedSize) {
							newData := make([]byte, fileData.CompressedSize)
							copy(newData, zipEntr.Data)
							for k := len(newData); k < int(fileData.CompressedSize); k++ {
								newData[k] = 0
							}
							swReader.WriteAt(newData, int64(fileData.Offset)+int64(fileData.HeaderSize))
							hasInserted = true
						}
					} else if fileData.CompressionMethod == 0 {
						if fChng.IsCompressed {
							zipEntr, _ = relInfo.ZipReader.ParseZipFile(fChng.Data.File)
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
							for k := len(newData); k < int(fileData.UnCompressedSize); k++ {
								newData[k] = 0
							}
							swReader.WriteAt(newData, int64(fileData.Offset)+int64(fileData.HeaderSize))
							hasInserted = true
						}
					} else {
						log.Panicln("Expected 0 or 1 but got", fileData.CompressionMethod)
					}

					fmt.Println(relInfo.NumSuccessful, relInfo.NumChanges)
					if !hasInserted {
						runAfter = append(runAfter, fileData)
						runAfterZipEntrs = append(runAfterZipEntrs, zipEntr)
					} else {
						if relInfo.NumFilesSuccessful == relInfo.NumFileChanges {
							break
						}
					}
				}
			} else {
				relInfo.FilesNoHash++
			}

			swReader.Seek(restorePos, 0)
		}
		if tempTableOffset == 0 {
			archive.LastTableOffset = int64(fileTableOffset)
			archive.LastTableNumFiles = lastFile
		}
		fileTableOffset = tempTableOffset
	}

	for k, fileData := range runAfter {
		zipEntr := runAfterZipEntrs[k]
		if archive.LastTableNumFiles+1 >= 1000 {
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
			swReader.WriteAt(byteOffset, archive.LastTableOffset+4)

			archive.LastTableNumFiles = -1
			archive.LastTableOffset = newLastOffset
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
		archive.LastTableNumFiles++
		swReader.Seek(archive.LastTableOffset+12+int64(34*archive.LastTableNumFiles), 0)

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
}
func readNodeTor(torName string, nodeHashes map[string]bool) {
	if relInfo.NumNodeChanges == 0 {
		return
	}

	archive := TorArchiveStruct{}
	archive.Name = torName
	f, err := os.OpenFile(torName, os.O_RDWR, os.ModePerm)

	defer f.Close()
	logger.Check(err)

	swReader := reader.SWTORReader{File: f}
	magicNumber := swReader.ReadUInt32()

	if magicNumber != 0x50594D {
		fmt.Println("Not MYP File")
	}

	f.Seek(12, 0)

	fileTableOffset := swReader.ReadUInt64()

	namedFiles := 0

	hasBackedUp := false

	for fileTableOffset != 0 {
		f.Seek(int64(fileTableOffset), 0)
		numFiles := int32(swReader.ReadUInt32())
		fileTableOffset = swReader.ReadUInt64()
		namedFiles += int(numFiles)
		placeHolder := TorFile{}
		placeHolder.TorFile = torName
		for i := int32(0); i < numFiles; i++ {
			//fmt.Println(i, numFiles)
			offset := swReader.ReadUInt64()
			if offset == 0 {
				f.Seek(26, 1)
				continue
			}
			nData := TorFile{}
			nData.HeaderSize = swReader.ReadUInt32()
			nData.Offset = offset
			nData.CompressedSize = swReader.ReadUInt32()
			nData.UnCompressedSize = swReader.ReadUInt32()
			current_position, _ := f.Seek(0, 1)
			nData.SecondaryHash = swReader.ReadUInt32()
			nData.PrimaryHash = swReader.ReadUInt32()
			f.Seek(current_position, 0)
			nData.FileID = swReader.ReadUInt64()
			nData.Checksum = swReader.ReadUInt32()
			nData.CompressionMethod = swReader.ReadUInt16()
			nData.CRC = nData.Checksum
			nData.TorFile = torName
			key := strconv.Itoa(int(nData.PrimaryHash)) + "|" + strconv.Itoa(int(nData.SecondaryHash))

			oldPos, _ := swReader.Seek(0, 1)

			if relInfo.NumNodeChanges != 0 {
				if relInfo.BackupObj.Backup && !hasBackedUp {
					fCopy(archive.Name, relInfo.BackupObj.Path+archive.Name[strings.LastIndex(archive.Name, "\\"):])
				}

				if _, ok := nodeHashes[key]; ok {
					relInfo.FilesAttempted++

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
						if nChng, fndC := checkNodeMatches(relInfo.FileChanges.Nodes, gomName); fndC {
							//fmt.Println("Found Node!")
							relInfo.NumSuccessful++
							relInfo.NumNodesSuccessful++

							//replace node here
							var zipEntr reader.ZipEntry
							if nChng.IsCompressed {
								zipEntr, _ = relInfo.ZipReader.ParseZipNode(nChng.Data.File)
								var err5 error
								zipEntr.Data, err5 = zlipCompress(zipEntr.Data, relInfo.TmpIdxSub, nChng.Data.File[strings.LastIndex(nChng.Data.File, "\\")+1:], relInfo.ComprCmd)
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

								compressed, err5 := zlipCompress(uncomprData, relInfo.TmpIdxSub, nChng.Data.File[strings.LastIndex(nChng.Data.File, "\\")+1:], relInfo.ComprCmd)
								if err5 != nil {
									log.Panicln(err5)
								}
								zipEntr = reader.ZipEntry{Name: gomName, Data: compressed, CompressedSize: int64(len(compressed)), UncompressedSize: uncomprSize}
							}
							if int(zipEntr.CompressedSize) > int(nodeSize)-int(dataOffset) {
								log.Panicln("Custom node is too large. Needs to be the same size or smaller.")
								if relInfo.NumNodesSuccessful == relInfo.NumNodeChanges {
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

							if relInfo.NumNodesSuccessful == relInfo.NumNodeChanges {
								swReader.Seek(nodeOffset+((int64(nodeSize)+7)&-8), 0)
								break
							}
						}
						swReader.Seek(nodeOffset+((int64(nodeSize)+7)&-8), 0)
					}

					swReader.Seek(oldPos, 0)
					fmt.Println(relInfo.NumSuccessful, relInfo.NumChanges)
					if relInfo.NumNodesSuccessful == relInfo.NumNodeChanges {
						break
					}
				} else {
					relInfo.FilesNoHash++
				}
			}

			swReader.Seek(oldPos, 0)
		}
	}
}
