package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Tormak9970/file-changer/reader/hash"
	"github.com/Tormak9970/file-changer/reader/tor"
)

//* Build Command: go build -o fileChanger.exe main.go

func preprocessZip(changes *tor.Changes, zipPath string, destDir string) error {
	zipFile, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	os.MkdirAll(destDir, os.ModePerm)

	for _, file := range zipFile.File {
		newPath := filepath.Join(destDir, file.Name)
		f, err2 := file.Open()
		if err2 != nil {
			return err
		}

		if !strings.HasPrefix(newPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return errors.New("invalid file path")
		}
		if file.FileInfo().IsDir() {
			os.MkdirAll(newPath, os.ModePerm)
			continue
		}

		err3 := os.MkdirAll(filepath.Dir(newPath), os.ModePerm)
		if err3 != nil {
			return err3
		}

		contents := make([]byte, file.UncompressedSize64)
		f.Read(contents)

		err4 := os.WriteFile(newPath, contents, os.ModePerm)
		if err4 != nil {
			return err4
		}

		err5 := updateChanges(changes, newPath)
		if err5 != nil {
			return err5
		}
	}

	return nil
}

func updateChanges(changes *tor.Changes, newPath string) error {
	if filepath.Ext(newPath) == ".node" {
		for i, node := range (*changes).Nodes {
			if filepath.Base(node.Data.File) == filepath.Base(newPath) {
				(*changes).Nodes[i].Data.File = newPath
				break
			}
		}
	} else {
		for i, file := range (*changes).Files {
			if filepath.Base(file.Data.File) == filepath.Base(newPath) {
				(*changes).Files[i].Data.File = newPath
				break
			}
		}
	}

	return nil
}

func cleanUpZip(cleanUpTarget string) error {
	err := os.RemoveAll(cleanUpTarget)
	return err
}

func main() {
	var torFiles []string
	var backupObj tor.BackupObj
	var fileChanges tor.Changes

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

		changesJson := os.Args[4]
		changesDat, err25 := os.ReadFile(changesJson)
		if err25 != nil {
			fmt.Println(err25)
		}
		err3 := json.Unmarshal(changesDat, &fileChanges)
		if err3 != nil {
			fmt.Println(err3)
		}
		os.Remove(changesJson)

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

	nodeHashes := make(map[string]bool)
	for i := 0; i < 500; i++ {
		fileName := "/resources/systemgenerated/buckets/" + strconv.Itoa(i) + ".bkt"
		litHashes := hash.FromFilePath(fileName, 0)
		key := strconv.Itoa(int(litHashes.PH)) + "|" + strconv.Itoa(int(litHashes.SH))
		nodeHashes[key] = true
	}

	hashes := hash.Read(hashPath)
	lastIdxSub := backupObj.Path[0:strings.LastIndex(backupObj.Path, "\\")]
	tmpIdxSub := lastIdxSub[0:strings.LastIndex(lastIdxSub, "\\")+1] + "tmp"

	var cleanUpTarget string
	if zipFilePath != "" {
		zBase := filepath.Base(zipFilePath)
		cleanUpTarget = filepath.Join(tmpIdxSub, zBase[0:strings.LastIndex(zBase, ".")])

		err := preprocessZip(&fileChanges, zipFilePath, cleanUpTarget)
		if err != nil {
			log.Panicln(err)
		}
	}

	relInf := tor.RelivantInfo{BackupObj: backupObj, FileChanges: fileChanges, ComprCmd: comprCmd, FilesNoHash: 0, FilesAttempted: 0, NumNodeChanges: len(fileChanges.Nodes), NumFileChanges: len(fileChanges.Files), NumChanges: len(fileChanges.Nodes) + len(fileChanges.Files), NumNodesSuccessful: 0, NumFilesSuccessful: 0, NumSuccessful: 0, TmpIdxSub: tmpIdxSub}

	s1 := time.Now()
	relInf = tor.ReadAll(torFiles, hashes, nodeHashes, relInf)
	d1 := time.Since(s1)
	log.Println("duration", fmt.Sprintf("%s", d1))

	if zipFilePath != "" {
		err := cleanUpZip(cleanUpTarget)
		if err != nil {
			log.Panicln(err)
		}
	}

	if relInf.NumSuccessful == relInf.NumChanges {
		os.Exit(0)
	} else {
		os.Exit(1)
	}
}
