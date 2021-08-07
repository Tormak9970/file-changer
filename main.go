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
	"path/filepath"
	"strconv"
	"strings"
	"time"

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
func checkNodeMatches(changes []tor.NodeChange, gomName string) (tor.NodeChange, bool) {
	for _, change := range changes {
		if change.Name == gomName {
			return change, true
		}
	}
	return tor.NodeChange{}, false
}

func main() {
	var torFiles []string
	var backupObj tor.BackupObj
	var fileChanges tor.Changes

	hashPath := ""
	zipFilePath := ""
	comprCmd := ""
	if len(os.Args) >= 5 {
		// err := json.Unmarshal([]byte(os.Args[1]), &torFiles)
		// if err != nil {
		// 	fmt.Println(err)
		// }
		tempRes := string([]byte(os.Args[1]))
		torFiles = append(torFiles, tempRes)

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

	if len(torFiles) == 1 {
		torName := torFiles[0]
		torFiles = []string{}

		f, _ := os.Open(torName)
		fi, _ := f.Stat()

		switch mode := fi.Mode(); {
		case mode.IsDir():
			files, _ := ioutil.ReadDir(torName)

			for _, f := range files {
				file := filepath.Join(torName, f.Name())

				fileMode := f.Mode()

				if fileMode.IsRegular() {
					if filepath.Ext(file) == ".tor" {
						torFiles = append(torFiles, file)
					}
				}
			}
		case mode.IsRegular():
			torFiles = append(torFiles, torName)
		}
	}

	hashes := hash.Read(hashPath)
	lastIdxSub := backupObj.Path[0:strings.LastIndex(backupObj.Path, "\\")]
	tmpIdxSub := lastIdxSub[0:strings.LastIndex(lastIdxSub, "\\")+1] + "tmp"

	var zipReader reader.InMemoryZip
	if zipFilePath != "" {
		zipReader = reader.ReadZip(zipFilePath)
	}
	fmt.Println(zipReader)

	filesNoHash := 0

	filesAttempted := 0
	start := time.Now()

	numNodeChanges := len(fileChanges.Nodes)
	numFileChanges := len(fileChanges.Files)
	numChanges := numFileChanges + numNodeChanges
	numNodesSuccessful := 0
	//numFilesSuccessful := 0
	numSuccessful := 0

	relInf := tor.RelivantInfo{BackupObj: backupObj, FileChanges: fileChanges, ComprCmd: comprCmd, ZipReader: zipReader, FilesNoHash: 0, FilesAttempted: 0, NumNodeChanges: len(fileChanges.Nodes), NumFileChanges: len(fileChanges.Files), NumChanges: len(fileChanges.Nodes) + len(fileChanges.Files), NumNodesSuccessful: 0, NumFilesSuccessful: 0, NumSuccessful: 0, TmpIdxSub: tmpIdxSub}

	s1 := time.Now()
	data, nodeData := tor.ReadAll(torFiles, hashes, relInf)
	d1 := time.Now().Sub(s1)
	log.Println("duration", fmt.Sprintf("%s", d1))

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
