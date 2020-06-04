package main

import (
	"fmt"
	"github.com/justinas/nosurf"
	"github.com/xieyicai/render/render"
	"log"
	"net/http"
	"time"
)

func main() {
	fileHandler := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fileHandler)) // 启动静态文件服务
	http.HandleFunc("/", func(writer http.ResponseWriter, req *http.Request) {
		if req.URL.Path=="/favicon.ico" {
			fileHandler.ServeHTTP(writer, req)
		}else{
			render.Out(writer, req, nil)
		}
	})
	render.DefaultData = render.DataFunc(func(writer http.ResponseWriter, req *http.Request) interface{} {
		map1 := make(map[string]string)
		map1["Title"]="Untitled Document"
		map1["CsrfToken"]=nosurf.Token(req)
		return &map1
	})
	render.SetData("/index.html", &struct {
		Title string
	}{
		Title: "品牌街-上天猫，就够了",
	})
	render.SetDataFunc("/test2.html", func(writer http.ResponseWriter, req *http.Request) interface{} {
		map1 := make(map[string]string)
		map1["Title"]=fmt.Sprintf("当前时间是：%s", time.Now().Format("2006-01-02 15:04:05"))
		map1["CsrfToken"]=nosurf.Token(req)
		return &map1
	})
	defer render.TurnOff()
	render.TurnOn()
	log.Println("http://localhost:8080/")
	log.Fatal(http.ListenAndServe(":8080", nosurf.New(http.DefaultServeMux)))
}
