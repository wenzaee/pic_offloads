package task

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// 压缩文件夹为 tar 格式
func CompressFolderToTar(srcDir string) ([]byte, error) {
	buf := new(bytes.Buffer)
	tarWriter := tar.NewWriter(buf)

	err := filepath.Walk(srcDir, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}

		//创建文件的tar头
		header, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		header.Name = strings.TrimPrefix(file, srcDir+"/")
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// 写入文件内容
		fileContent, err := os.Open(file)
		if err != nil {
			return err
		}
		defer fileContent.Close()
		_, err = io.Copy(tarWriter, fileContent)
		return err
	})
	if err != nil {
		return nil, err
	}

	if err := tarWriter.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// 解压 tar 格式文件到指定目录
func ExtractTarToFolder(tarData []byte, destDir string) error {
	// 创建目标目录
	err := os.MkdirAll(destDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("创建目录失败: %v", err)
	}

	tarReader := tar.NewReader(bytes.NewReader(tarData))

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// 构建解压路径
		filePath := filepath.Join(destDir, header.Name)
		log.Println("Extracting:", filePath) // 打印解压文件的路径

		// 确保解压目录存在
		dirPath := filepath.Dir(filePath)
		err = os.MkdirAll(dirPath, os.ModePerm)
		if err != nil {
			log.Printf("创建解压目录失败: %s, 错误: %v\n", dirPath, err)
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			// 创建目录
			if err := os.MkdirAll(filePath, os.ModePerm); err != nil {
				return err
			}
		case tar.TypeReg:
			// 创建文件
			fileContent, err := os.Create(filePath)
			if err != nil {
				log.Printf("无法创建文件 %s: %v\n", filePath, err)
				return err
			}
			defer fileContent.Close()

			// 将内容写入文件
			if _, err := io.Copy(fileContent, tarReader); err != nil {
				log.Printf("写入文件 %s 时出错: %v\n", filePath, err)
				return err
			}
		}
	}
	return nil
}
