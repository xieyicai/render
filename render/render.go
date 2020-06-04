package render

import (
	"errors"
	"fmt"
	"github.com/justinas/nosurf"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	Dir = "templates"
	Suffix = ".html"
	ContextPath = ""
	Sleep = time.Duration(10)*time.Second
	cache = make(map[string]os.FileInfo)
	tt *template.Template
	Pause = true
	DataMap = make(map[string]DataHandler)
	DefaultData DataHandler
	run = make(chan bool, 10)
)
type DataHandler interface {
	GetData(http.ResponseWriter, *http.Request) interface{}
}

type DataFunc func(http.ResponseWriter, *http.Request) interface{}
func (dh DataFunc) GetData(writer http.ResponseWriter, req *http.Request) interface{} {
	return dh(writer, req)
}

type Data struct {
	d interface{}
}
func (dh Data) GetData(http.ResponseWriter, *http.Request) interface{} {
	return dh.d
}

func init(){
	go func() {
		for {
			time.Sleep(Sleep)
			if run==nil {
				break
			}else if !Pause {
				run <- true		//每隔10秒钟，扫描一次文件
			}
		}
	}()
	go func() {
		for {
			if ! <- run {	// 如果从通道中接收到的信号是 false，表示关闭，就退出循环。
				run=nil
				break
			}else{
				if IsChange() {		//文件一旦发生改变，就要重载所有的模板，因为模板一旦被执行后，就不能再次解析，所以无法做到单个文件的热加载。只能重新解析全部的模板文件
					tt, cache = LoadTemplates()
				}
			}
		}
	}()
}
func TurnOn(){
	if run==nil {
		panic("已经关闭，无法重启。")
	}else{
		Pause=false
		run <- true
	}
}
func TurnOff(){
	Pause=true
	if run!=nil {
		run <- false
	}
}
func IsChange() bool {
	if tt==nil {
		return true
	}
	var Changed = errors.New("file was changed")
	err:=filepath.Walk(Dir, func (path string, info os.FileInfo, err error) error {
		if err==nil && !info.IsDir() {
			old, exists:=cache[path]
			if !exists || old.ModTime()!=info.ModTime() || old.Size()!=info.Size() {
				//fmt.Printf("文件发生改变：exists=%v\r\n%+v\r\n%+v\r\n", exists, old, info)
				return Changed
			}
			//fmt.Printf("文件没有发生改变：%s。\r\n", path)
		}
		return nil
	})
	return err==Changed
}
func LoadTemplates() (*template.Template, map[string]os.FileInfo) {
	var all *template.Template
	map1 := make(map[string]os.FileInfo)
	err:=filepath.Walk(Dir, func (path string, info os.FileInfo, err error) error {
		if err==nil {
			if !info.IsDir() && strings.HasSuffix(info.Name(), Suffix) {
				b, err := ioutil.ReadFile(path)
				if err != nil {
					return err
				}
				s := string(b)
				name:=filepath.ToSlash(path[len(Dir):])
				if all==nil {
					all, err = template.New(name).Parse(s)
				}else{
					_, err = all.New(name).Parse(s)
				}
				if err!=nil {
					fmt.Printf("模板文件存在语法错误：%s， %v。\r\n", path, err)
				}else{
					fmt.Printf("成功解析模板：%s。\r\n", path)
					map1[path]=info
				}
			}
		}else{
			fmt.Printf("无法访问文件：%s，%v\r\n", path, err)
		}
		return nil
	})
	if err!=nil {
		fmt.Printf("无法遍历目录：%v\r\n", err)
	}
	return all, map1
}
// 向客户端发送HTML格式的错误信息
func SendError(writer http.ResponseWriter, req *http.Request, statusCode int, title string, description interface{}) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(statusCode)
	filename:=fmt.Sprintf("/errors/%d.html", statusCode)
	_, exists:=cache[filepath.Join(Dir, filename)]
	if exists {
		var data interface{}
		obj, has := DataMap[filename]
		if has {
			data = obj.GetData(writer, req)
		}
		if data==nil {
			map1:=make(map[string]interface{})
			map1["Title"]=title
			map1["Description"]=description
			map1["CsrfToken"]=nosurf.Token(req)
			data=&map1
		}else{
			if map1, yes := data.(map[string]interface{}); yes {
				map1["Title"]=title
				map1["Description"]=description
				map1["CsrfToken"]=nosurf.Token(req)
			}
		}
		if err := tt.ExecuteTemplate(writer, filename, data); err != nil {
			log.Printf("模板渲染失败，未能向客户端发送错误信息。 %v.\r\n", err)
		}
	}else{
		if _, err := fmt.Fprintf(writer, "<html><body><h2>%s</h2><pre>%v</pre></body></html>", strings.ReplaceAll(title, "<", "&lt;"), description); err != nil { //向浏览器输出
			log.Printf("The connection to the client has been disconnected. %v.\r\n", err)
		}
	}
}
/*
模板渲染
writer		输出
req			输入
data		渲染模板所需的参数（map or struct）
filenames	需要解析的模板文件名，第一个将用来渲染
*/
func Out(writer http.ResponseWriter, req *http.Request, data interface{}, filenames... string) {
	var filename string
	if len(filenames)==0 {
		filename = req.URL.Path
		if filename[len(filename)-1] == '/' {
			filename = filename + "index.html"
		}else if strings.Contains(filename, "..") {
			SendError(writer, req, http.StatusBadRequest, "非法的请求", "请求地址含有非法的字符。")
			return
		}
		filename = filename[len(ContextPath):]
	}else{
		filename = filenames[0]
	}
	_, exists:=cache[filepath.Join(Dir, filename)]
	if exists {
		if data==nil {
			data = GetData(filename, writer, req)
		}
		if err := tt.ExecuteTemplate(writer, filename, data); err != nil {
			SendError(writer, req, http.StatusInternalServerError, "模板渲染失败", fmt.Sprintf("模板渲染失败：%v", err))
		}
	}else{
		SendError(writer, req, http.StatusNotFound, "您要访问的资源不存在", fmt.Sprintf("不存在指定的模板文件：%s。", filename))
	}
}
func SetData(path string, data interface{}){
	DataMap[path]=&Data{d:data}
}
func SetDataFunc(path string, handler func(writer http.ResponseWriter, req *http.Request) interface{}){
	DataMap[path]=DataFunc(handler)
}
func GetData(path string, writer http.ResponseWriter, req *http.Request) interface{} {
	obj, exists := DataMap[path]
	if exists {
		return obj.GetData(writer, req)
	}
	return DefaultData.GetData(writer, req)
}