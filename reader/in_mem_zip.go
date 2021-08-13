package reader

type ZipEntry struct {
	Name             string
	Data             []byte
	CompressedSize   int64
	UncompressedSize int64
}
