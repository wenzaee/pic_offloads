package task

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/libp2p/go-libp2p/core/network"
	"golang.org/x/net/context"
	"io"
	"log"
	"os"
	"path/filepath"
	deafault "pic_offload/pkg/apis"
	"strings"
)

func (ts *TaskScheduler) TransferTaskToTargetHost(taskstr, TargetName string) error {
	task := ts.Tasks[taskstr]
	fmt.Println("task", task, "targetname", TargetName)
	tarData, err := CompressFolderToTar(task.FilePath)
	if err != nil {
		return fmt.Errorf("压缩文件夹失败: %v", err)
	}

	Targetpid := ts.registry.MapNamePeer[TargetName]
	stream, err := ts.h.NewStream(context.Background(), Targetpid, deafault.FilesProtocol)
	fmt.Println("task!2", task, "targetname", TargetName)
	if err != nil {
		fmt.Println(err)
		return fmt.Errorf("无法建立流连接: %v", err)
	}
	fmt.Println("!!!task!3", task, "targetname", TargetName)
	defer stream.Close()
	fmt.Println("task!3", task, "targetname", TargetName)
	ts.Tasks[taskstr].Hostname = TargetName
	fmt.Println("task!4", task, "targetname", TargetName)
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
	baseDir := "/root/pic_offloads/worker"
	subDirName := task.Tasktype + "-" + task.ID
	targetDir := filepath.Join(baseDir, subDirName)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		log.Printf("创建目标目录失败: %v", err)
		return
	}

	// 解压接收到的 tar 文件
	err = ExtractTarToFolder(tarData, targetDir)
	if err != nil {
		log.Printf("解压失败: %v", err)
	} else {
		log.Println("文件接收并解压成功")
	}

}

func (ts *TaskScheduler) RequestTaskMigration(taskID, targetHost string) error {
	// 获取当前任务
	task, exists := ts.Tasks[taskID]
	if !exists {
		return fmt.Errorf("任务ID %s 不存在", taskID)
	}
	log.Println("将要转移", task, "至", targetHost)
	// 生成任务迁移请求
	migrationRequest := TaskMigrationRequest{
		TaskID:      taskID,
		SourceHost:  task.Hostname,
		TargetHost:  targetHost,
		TaskDetails: *task,
	}

	// 获取目标主机的 Peer ID
	sourcePID := ts.registry.MapNamePeer[migrationRequest.SourceHost]
	if sourcePID == "" {
		return fmt.Errorf("目标主机 %s 的 Peer ID 不存在", targetHost)
	}

	stream, err := ts.h.NewStream(context.Background(), sourcePID, deafault.RequestProtocol)
	if err != nil {
		return fmt.Errorf("无法建立流连接到目标主机: %v", err)
	}
	defer stream.Close()

	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	err = encoder.Encode(migrationRequest)
	if err != nil {
		return fmt.Errorf("迁移请求编码失败: %v", err)
	}

	// 发送迁移请求数据到目标主机
	_, err = stream.Write(buf.Bytes())
	if err != nil {
		return fmt.Errorf("迁移请求发送失败: %v", err)
	}

	// 输出迁移请求成功消息
	fmt.Printf("任务 %s 成功请求迁移到目标主机 %s\n", taskID, targetHost)
	return nil
}
func (ts *TaskScheduler) HandleRequestTask(s network.Stream) {
	var taskRequest TaskMigrationRequest
	decoder := gob.NewDecoder(s)
	err := decoder.Decode(&taskRequest)
	if err != nil {
		log.Printf("解码失败: %v", err)

	}
	log.Println("收到一个Request", taskRequest)
	err = ts.TransferTaskToTargetHost(taskRequest.TaskDetails.ID, taskRequest.TargetHost)
	if err != nil {
		return
	}
	log.Println("成功转移")
}
func (ts *TaskScheduler) SendTask(Taskid, Target string) error {

	TargetPID := ts.registry.MapNamePeer[Target]
	stream, err := ts.h.NewStream(context.Background(), TargetPID, deafault.SendTaskProtocal)
	if err != nil {
		return fmt.Errorf("无法建立流连接到目标主机: %v", err)
	}
	defer stream.Close()

	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	err = encoder.Encode(ts.Tasks[Taskid])
	if err != nil {
		return fmt.Errorf("发送Task编码失败: %v", err)
	}

	// 发送迁移请求数据到目标主机
	_, err = stream.Write(buf.Bytes())
	if err != nil {
		return fmt.Errorf("发送Task信息失败: %v", err)
	}
	return nil
}
func (ts *TaskScheduler) HandleSendTask(s network.Stream) {
	var task Task
	decoder := gob.NewDecoder(s)
	err := decoder.Decode(&task)
	if err != nil {
		log.Printf("解码失败: %v", err)

	}
	log.Println("收到一个task", task)
	ts.Tasks[task.ID] = &task
	return
}
func (ts *TaskScheduler) AskTaskDone(Taskid string) {
	TargetName := ts.Tasks[Taskid].Hostname
	TargetPID := ts.registry.MapNamePeer[TargetName]

	stream, err := ts.h.NewStream(context.Background(), TargetPID, deafault.AskProtocol)
	if err != nil {
		log.Fatal(err)
	}
	defer stream.Close()

	// 发送键（Key）
	key := Taskid + "\n"
	if _, err := stream.Write([]byte(key)); err != nil {
		log.Fatal(err)
	}
	value, _ := bufio.NewReader(stream).ReadString('\n')
	fmt.Printf("Received value: %s", value)
	value = strings.TrimSuffix(value, "\n")
	if value == "true" {
		ts.Tasks[Taskid].Done = true
	}

}

func (ts *TaskScheduler) HandleAskTask(s network.Stream) {
	value, _ := bufio.NewReader(s).ReadString('\n')
	value = strings.TrimSuffix(value, "\n")
	fmt.Printf("Received value: !%s!", value)
	var res string
	if ts.Tasks[value].Done == true {
		res = "true\n"
	} else {
		res = "false\n"
	}

	if _, err := s.Write([]byte(res)); err != nil {
		log.Printf("Error writing response: %v", err)
	}

}
