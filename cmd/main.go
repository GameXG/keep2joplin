package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const outTemplate = `---
title: %v
updated: %v
created: %v
---

%v`

func main() {
	argsSrcDir := flag.String("src", "Google Keep", "google keep dir")
	argsOutDir := flag.String("out", "keep", "output directory")

	flag.Parse()

	srcDir, err := filepath.Abs(*argsSrcDir)
	if err != nil {
		log.Fatalf("abs, %v", err)
	}

	outDir, err := filepath.Abs(*argsOutDir)
	if err != nil {
		log.Fatalf("abs, %v", err)
	}

	_ = os.MkdirAll(outDir, 0770)

	fPathList, err := filepath.Glob(filepath.Join(srcDir, "*.json"))
	if err != nil {
		log.Fatalf("glob, %v", err)
	}

	for _, v := range fPathList {
		outFilePath := filepath.Join(outDir, fmt.Sprintf("%v.md", strings.TrimSuffix(filepath.Base(v), filepath.Ext(v))))

		err := convert(v, outFilePath)
		if err != nil {
			log.Printf("convert(%v), %v", err)
		}
	}
}

func convert(srcFilePath, dstFilePath string) error {
	sFile, err := os.Open(srcFilePath)
	if err != nil {
		return fmt.Errorf("os.Openm %v", err)
	}
	defer sFile.Close()

	dec := json.NewDecoder(sFile)

	i := Item{}
	err = dec.Decode(&i)
	if err != nil {
		return fmt.Errorf("json, %v", err)
	}

	textContentList := make([]string, 0, 1+len(i.ListContent)+1+len(i.Annotations)+1+len(i.Attachments)+1)

	// 内容主体
	if len(i.TextContent) > 0 {
		textContent := strings.ReplaceAll(i.TextContent, "\n", "\n\n")
		textContentList = append(textContentList, fmt.Sprintf("%v", textContent))
	}

	// 多选项
	if len(i.ListContent) > 0 {
		if len(textContentList) > 0 {
			textContentList = append(textContentList, "\r\n* * *\r\n")
		}
		for _, v := range i.ListContent {
			IsChecked := " "
			if v.IsChecked {
				IsChecked = "x"
			}
			textContentList = append(textContentList, fmt.Sprintf("- [%v] %v\r\n", IsChecked, strings.ReplaceAll(v.Text, "\n", "\t")))
		}
	}

	// 附加链接
	if len(i.Annotations) > 0 {
		if len(textContentList) > 0 {
			textContentList = append(textContentList, "\r\n* * *\r\n")
		}
		for _, v := range i.Annotations {
			switch strings.ToUpper(v.Source) {
			case "WEBLINK":
				title := v.Title
				title = strings.ReplaceAll(title, "\n", "\t")
				title = strings.ReplaceAll(title, "[", "_")
				title = strings.ReplaceAll(title, "]", "_")

				_url := v.Url
				textContentList = append(textContentList, fmt.Sprintf("[%v](%v)\r\n\r\n", title, _url))
			default:
				return fmt.Errorf("unexpected annotations.source %v", v.Source)
			}
		}
	}

	// 附件
	if len(i.Attachments) > 0 {
		if len(textContentList) > 0 {
			textContentList = append(textContentList, "\r\n* * *\r\n")
		}

		attachmentsDir := filepath.Join(filepath.Dir(dstFilePath), "_resources")
		_ = os.MkdirAll(attachmentsDir, 0770)

		for _, v := range i.Attachments {
			ml := strings.SplitN(v.Mimetype, "/", 2)
			switch strings.ToLower(ml[0]) {
			case "image", "audio":
				imgAbsPath := filepath.Join(filepath.Dir(srcFilePath), v.FilePath)
				if Exists(imgAbsPath) == false {
					// 修复导出路径错误
					if filepath.Ext(imgAbsPath) == ".jpeg" {
						imgAbsPath = fmt.Sprintf("%v%v", strings.TrimSuffix(imgAbsPath, ".jpeg"), ".jpg")
					}

					if Exists(imgAbsPath) == false {
						log.Fatalf("file not found, %v", imgAbsPath)
					}
				}

				newImgAbsPAth := filepath.Join(attachmentsDir, filepath.Base(imgAbsPath))
				err := CopyFile(imgAbsPath, newImgAbsPAth)
				if err != nil {
					log.Fatalf("CopyFile, %v", err)
				}

				imgPath, err := filepath.Rel(filepath.Dir(dstFilePath), newImgAbsPAth)
				if err != nil {
					imgPath = imgAbsPath
				}

				textContentList = append(textContentList, fmt.Sprintf("![%v](%v)\r\n\r\n", filepath.Base(imgPath), strings.ReplaceAll(imgPath, "/", "\\")))

			default:
				log.Fatalf("unexpected Attachments.Mimetype, %v", v.Mimetype)
			}
		}
	}

	title := strings.ReplaceAll(i.Title, "\n", "\t")
	if len(title) == 0 {
		if len(title) == 0 && len(i.TextContent) > 0 {
			title = strings.TrimSpace(i.TextContent)
		}
		if len(title) == 0 && len(i.ListContent) > 0 {
			title = i.ListContent[0].Text
		}
		if len(title) == 0 && len(i.Annotations) > 0 {
			title = i.Annotations[0].Title
		}
		if len(title) == 0 && len(i.Attachments) > 0 {
			title = filepath.Base(i.Attachments[0].FilePath)
		}

		tList := strings.SplitN(title, "\n", 2)
		title = strings.TrimSpace(tList[0])
	}
	for _, v := range []string{"{", "}", "[", "]", ":", "?", "\""} {
		title = strings.ReplaceAll(title, v, "_")
	}

	cTime := time.Unix(0, i.CreatedTimestampUsec*1000).In(time.UTC)
	uTime := time.Unix(0, i.UserEditedTimestampUsec*1000).In(time.UTC)

	cTimeStr := cTime.Format("2006-01-02 15:04:05Z")
	uTimeStr := uTime.Format("2006-01-02 15:04:05Z")

	outTxt := fmt.Sprintf(outTemplate, title, uTimeStr, cTimeStr, strings.Join(textContentList, ""))

	err = ioutil.WriteFile(dstFilePath, []byte(outTxt), 0644)
	if err != nil {
		return err
	}
	
	err = os.Chtimes(dstFilePath, cTime, uTime )
	if err != nil {
		return err
	}

	return nil
}

type Item struct {
	// 附件：移动光猫配置.json
	Attachments []struct {
		FilePath string `json:"filePath"`
		Mimetype string `json:"mimetype"`
	} `json:"attachments"`
	Color      string `json:"color"`
	IsTrashed  bool   `json:"isTrashed"`
	IsPinned   bool   `json:"isPinned"`
	IsArchived bool   `json:"isArchived"`
	// 附加链接时存在：例子：吉利帝豪GL汽车告诉您雨刮 举起手来_手机搜狐网.json
	Annotations []struct {
		Description string `json:"description"`
		Source      string `json:"source"`
		Title       string `json:"title"`
		Url         string `json:"url"`
	} `json:"annotations"`
	// 多选项时存在-例子：2018-05-10T10_08_10.510-04_00.json
	// 存在 List 时，看起来就不会有 TextContent 了
	ListContent []struct {
		Text      string `json:"text"`
		IsChecked bool   `json:"isChecked"`
	} `json:"listContent"`
	TextContent             string `json:"textContent"`
	Title                   string `json:"title"`
	UserEditedTimestampUsec int64  `json:"userEditedTimestampUsec"`
	CreatedTimestampUsec    int64  `json:"createdTimestampUsec"`
}

func Exists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func CopyFile(srcPath, dstPath string) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, srcFile)
	if err != nil {
		return err
	}

	err = dst.Close()
	if err != nil {
		return err
	}

	return nil
}
