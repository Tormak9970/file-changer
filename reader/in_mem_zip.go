package reader

import (
	"archive/zip"
	"bytes"
	"errors"
	"log"
	"os"
)

type InMemoryZip struct {
	ZReader *zip.ReadCloser
	OFile   *os.File
	Length  int64
}

type ZipEntry struct {
	Name             string
	Data             []byte
	CompressedSize   int64
	UncompressedSize int64
}

func ReadZip(archive string) InMemoryZip {
	z, _ := os.Open(archive)
	defer z.Close()

	zStat, _ := z.Stat()
	zSize := zStat.Size()
	zf, _ := zip.OpenReader(archive)
	return InMemoryZip{zf, z, zSize}
}

func (self InMemoryZip) ReadAt(offset int64, len int64) []byte {
	data := make([]byte, len)

	self.OFile.Seek(offset, 0)
	self.OFile.Read(data)

	return data
}

func (self InMemoryZip) ParseZipNode(targetFile string) (ZipEntry, error) {
	for _, file := range self.ZReader.File {
		if zFI := file.FileInfo(); zFI.Name() == targetFile {
			comprSize := int64(file.CompressedSize64)
			uncomprSize := int64(file.UncompressedSize64)
			f, err := file.Open()
			if err != nil {
				log.Panicln(err)
			}
			buf := new(bytes.Buffer)
			buf.ReadFrom(f)
			f.Close()
			return ZipEntry{zFI.Name(), buf.Bytes(), comprSize, uncomprSize}, nil
		}
	}
	return ZipEntry{}, errors.New("Could not find file " + targetFile + " in zip")
}

func (self InMemoryZip) ParseZipFile(targetFile string) (ZipEntry, error) {
	for _, file := range self.ZReader.File {
		if zFI := file.FileInfo(); zFI.Name() == targetFile {
			comprSize := int64(file.CompressedSize64)
			uncomprSize := int64(file.UncompressedSize64)
			f, err := file.Open()
			if err != nil {
				log.Panicln(err)
			}
			buf := new(bytes.Buffer)
			buf.ReadFrom(f)
			f.Close()
			return ZipEntry{zFI.Name(), buf.Bytes(), comprSize, uncomprSize}, nil
		}
	}
	return ZipEntry{}, errors.New("Could not find file " + targetFile + " in zip")
}
