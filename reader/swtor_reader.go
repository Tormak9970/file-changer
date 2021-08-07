package reader

import (
	"encoding/binary"
	"os"

	"github.com/Tormak9970/file-changer/logger"
)

type SWTORReader struct {
	File *os.File
}

func (self SWTORReader) Read(data []byte) (int64, error) {
	ret1, ret2 := self.File.Read(data)
	return int64(ret1), ret2
}

func (self SWTORReader) ReadUInt64() uint64 {
	bs := make([]byte, 8)
	_, err := self.File.Read(bs)
	logger.Check(err)

	return binary.LittleEndian.Uint64(bs)
}

func (self SWTORReader) ReadUInt16() uint16 {
	bs := make([]byte, 2)
	_, err := self.File.Read(bs)
	logger.Check(err)

	return binary.LittleEndian.Uint16(bs)
}

func (self SWTORReader) ReadUInt32() uint32 {
	bs := make([]byte, 4)
	_, err := self.File.Read(bs)
	logger.Check(err)

	return binary.LittleEndian.Uint32(bs)
}
func (self SWTORReader) ReadInt32() int32 {
	bs := make([]byte, 4)
	_, err := self.File.Read(bs)
	logger.Check(err)

	return int32(binary.LittleEndian.Uint32(bs))
}

func (self SWTORReader) Seek(offset int64, isRel int) (int64, error) {
	return self.File.Seek(offset, isRel)
}

func (self SWTORReader) WriteAt(data []byte, offset int64) (int64, error) {
	oldPos, _ := self.Seek(0, 1)
	ret1, ret2 := self.File.WriteAt(data, offset)
	self.Seek(oldPos, 0)
	return int64(ret1), ret2
}

func (self SWTORReader) Write(data []byte) (int64, error) {
	ret1, ret2 := self.File.Write(data)
	return int64(ret1), ret2
}
