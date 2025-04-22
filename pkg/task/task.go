package task

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/libp2p/go-libp2p/core/network"
	"golang.org/x/net/context"
	"io"
	"log"
	deafault "pic_offload/pkg/apis"
)

func (ts *TaskScheduler) TransferTaskToTargetHost(task *Task, TargetName string) error {
	tarData, err := CompressFolderToTar(task.FilePath)
	if err != nil {
		return fmt.Errorf("压缩文件夹失败: %v", err)
	}

	Targetpid := ts.registry.MapNamePeer[TargetName]
	stream, err := ts.h.NewStream(context.Background(), Targetpid, deafault.FilesProtocol)

	if err != nil {
		return fmt.Errorf("无法建立流连接: %v", err)
	}
	defer stream.Close()

	task.Hostname = TargetName

	var taskBuf bytes.Buffer
	encodertaskBuf := gob.NewEncoder(&taskBuf)
	err = encodertaskBuf.Encode(task)
	if err != nil {
		return fmt.Errorf("任务编码失败: %v", err)
	}
	_, err = stream.Write(taskBuf.Bytes())
	if err != nil {
		return fmt.Errorf("任务信息传输失败: %v", err)
	}

	_, err = stream.Write(tarData)
	if err != nil {
		return fmt.Errorf("文件传输失败: %v", err)
	}

	fmt.Printf("文件 %s 成功传输到目标节点 %s\n", task.FilePath)
	return nil
}
func (ts *TaskScheduler) Handlefiles(s network.Stream) {
	var task Task
	decoder := gob.NewDecoder(s)
	err := decoder.Decode(&task)
	if err != nil {
		log.Printf("任务解码失败: %v", err)
		return
	}
	ts.Tasks[task.ID] = &task
	// 输出接收到的任务信息
	fmt.Printf("接收到任务: ID = %s, Command = %s, Hostname = %s\n", task.ID, task.Command, task.Hostname)

	var buf [1024]byte
	tarData := []byte{}

	// 读取流中的数据
	for {
		n, err := s.Read(buf[:])
		if err != nil && err != io.EOF {
			log.Printf("读取流时出错: %v", err)
			break
		}
		if n == 0 {
			break
		}

		// 将数据追加到 tarData
		tarData = append(tarData, buf[:n]...)
	}
	// 解压接收到的 tar 文件
	err = ExtractTarToFolder(tarData, "/root/testget/")
	if err != nil {
		log.Printf("解压失败: %v", err)
	} else {
		log.Println("文件接收并解压成功")
	}

}
func (ts *TaskScheduler) Handleasks(s network.Stream) {
	
}
