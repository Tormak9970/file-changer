package tor

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/Tormak9970/file-changer/logger"
	"github.com/Tormak9970/file-changer/reader"
	"github.com/gammazero/workerpool"
)

var someMapMutex = sync.RWMutex{}

type torStruct struct {
	fileList []TorArchiveStruct
	mutex    sync.RWMutex
}
type TorArchiveStruct struct {
	Name              string
	FileList          []TorFile
	Offset            uint64
	NumTables         int
	LastTableOffset   int64
	LastTableNumFiles int
}
type nodeTorStruct struct {
	fileList map[string]TorFile
	mutex    sync.RWMutex
}

func (tor *torStruct) fileListAppend(data TorArchiveStruct) {
	tor.mutex.Lock()
	tor.fileList = append(tor.fileList, data)
	tor.mutex.Unlock()
}
func (tor *nodeTorStruct) NodeListAppend(key string, data TorFile) {
	tor.mutex.Lock()
	tor.fileList[key] = data
	tor.mutex.Unlock()
}

func ReadAll(torNames []string) ([]TorArchiveStruct, map[string]TorFile) {
	pool := workerpool.New(runtime.NumCPU())

	tor := torStruct{}
	nodeTor := nodeTorStruct{}

	for _, torName := range torNames {
		torName := torName

		if strings.Contains(torName, "main_global_1.tor") {
			pool.Submit(func() {
				readNodeTor(torName, &nodeTor, &tor)
			})
		} else {
			pool.Submit(func() {
				read(torName, &tor)
			})
		}
	}
	pool.StopWait()

	return tor.fileList, nodeTor.fileList
}

func Read(torName string) []TorArchiveStruct {
	tor := torStruct{}
	read(torName, &tor)
	return tor.fileList
}

func read(torName string, tor *torStruct) {
	archive := TorArchiveStruct{}
	archive.Name = torName
	f, err := os.Open(torName)

	defer f.Close()
	logger.Check(err)
	reader := reader.SWTORReader{File: f}
	magicNumber := reader.ReadUInt32()

	if magicNumber != 0x50594D {
		fmt.Println("Not MYP File")
	}

	f.Seek(12, 0)

	fileTableOffset := reader.ReadUInt64()
	archive.Offset = fileTableOffset

	namedFiles := 0
	lastFile := 0

	for fileTableOffset != 0 {
		archive.NumTables++
		f.Seek(int64(fileTableOffset), 0)
		numFiles := int32(reader.ReadUInt32())
		tempTableOffset := reader.ReadUInt64()
		namedFiles += int(numFiles)
		for i := int32(0); i < numFiles; i++ {
			debugOffset, _ := f.Seek(0, 1)
			offset := reader.ReadUInt64()
			if offset == 0 {
				f.Seek(26, 1)
				continue
			}
			info := TorFile{}
			info.HeaderOffset = debugOffset
			info.HeaderSize = reader.ReadUInt32()
			info.Offset = offset
			info.CompressedSize = reader.ReadUInt32()
			info.UnCompressedSize = reader.ReadUInt32()
			current_position, _ := f.Seek(0, 1)
			info.SecondaryHash = reader.ReadUInt32()
			info.PrimaryHash = reader.ReadUInt32()
			f.Seek(current_position, 0)
			info.FileID = reader.ReadUInt64()
			info.Checksum = reader.ReadUInt32()
			info.CompressionMethod = reader.ReadUInt16()
			info.CRC = info.Checksum
			info.TorFile = torName
			info.TableIdx = i
			archive.FileList = append(archive.FileList, info)
			lastFile = int(i)
		}
		if tempTableOffset == 0 {
			archive.LastTableOffset = int64(fileTableOffset)
			archive.LastTableNumFiles = lastFile
		}
		fileTableOffset = tempTableOffset
	}
	tor.fileListAppend(archive)
}
func readNodeTor(torName string, tor *nodeTorStruct, gTor *torStruct) {
	archive := TorArchiveStruct{}
	archive.Name = torName
	if tor.fileList == nil {
		tor.fileList = map[string]TorFile{}
	}
	f, err := os.Open(torName)

	defer f.Close()
	logger.Check(err)

	reader := reader.SWTORReader{File: f}
	magicNumber := reader.ReadUInt32()

	if magicNumber != 0x50594D {
		fmt.Println("Not MYP File")
	}

	f.Seek(12, 0)

	fileTableOffset := reader.ReadUInt64()

	namedFiles := 0

	for fileTableOffset != 0 {
		f.Seek(int64(fileTableOffset), 0)
		numFiles := int32(reader.ReadUInt32())
		fileTableOffset = reader.ReadUInt64()
		namedFiles += int(numFiles)
		placeHolder := TorFile{}
		placeHolder.TorFile = torName
		for i := int32(0); i < numFiles; i++ {
			//fmt.Println(i, numFiles)
			offset := reader.ReadUInt64()
			if offset == 0 {
				f.Seek(26, 1)
				continue
			}
			info := TorFile{}
			info.HeaderSize = reader.ReadUInt32()
			info.Offset = offset
			info.CompressedSize = reader.ReadUInt32()
			info.UnCompressedSize = reader.ReadUInt32()
			current_position, _ := f.Seek(0, 1)
			info.SecondaryHash = reader.ReadUInt32()
			info.PrimaryHash = reader.ReadUInt32()
			f.Seek(current_position, 0)
			info.FileID = reader.ReadUInt64()
			info.Checksum = reader.ReadUInt32()
			info.CompressionMethod = reader.ReadUInt16()
			info.CRC = info.Checksum
			info.TorFile = torName
			tor.NodeListAppend(strconv.Itoa(int(info.PrimaryHash))+"|"+strconv.Itoa(int(info.SecondaryHash)), info)
		}
	}
	gTor.fileListAppend(archive)
}
