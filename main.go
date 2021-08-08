package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Tormak9970/file-changer/reader"
	"github.com/Tormak9970/file-changer/reader/hash"
	"github.com/Tormak9970/file-changer/reader/tor"
)

//* Build Command: go build -o fileChanger.exe main.go

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

	var nodeHashes map[string]interface{}
	for i := 0; i < 500; i++ {
		fileName := "/resources/systemgenerated/buckets/" + strconv.Itoa(i) + ".bkt"
		litHashes := hash.FromFilePath(fileName, 0)
		key := strconv.Itoa(int(litHashes.PH)) + "|" + strconv.Itoa(int(litHashes.SH))
		nodeHashes[key] = true
	}

	hashes := hash.Read(hashPath)
	lastIdxSub := backupObj.Path[0:strings.LastIndex(backupObj.Path, "\\")]
	tmpIdxSub := lastIdxSub[0:strings.LastIndex(lastIdxSub, "\\")+1] + "tmp"

	var zipReader reader.InMemoryZip
	if zipFilePath != "" {
		zipReader = reader.ReadZip(zipFilePath)
	}

	relInf := tor.RelivantInfo{BackupObj: backupObj, FileChanges: fileChanges, ComprCmd: comprCmd, ZipReader: zipReader, FilesNoHash: 0, FilesAttempted: 0, NumNodeChanges: len(fileChanges.Nodes), NumFileChanges: len(fileChanges.Files), NumChanges: len(fileChanges.Nodes) + len(fileChanges.Files), NumNodesSuccessful: 0, NumFilesSuccessful: 0, NumSuccessful: 0, TmpIdxSub: tmpIdxSub}

	s1 := time.Now()
	tor.ReadAll(torFiles, hashes, nodeHashes, relInf)
	d1 := time.Now().Sub(s1)
	log.Println("duration", fmt.Sprintf("%s", d1))

	if relInf.NumSuccessful == relInf.NumChanges {
		os.Exit(0)
	} else {
		os.Exit(1)
	}
}
