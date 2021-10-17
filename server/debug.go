package server

import (
	"fmt"
	"html/template"
	"net/http"
)

const debugText = `
<html>
<body>
	<title>GoRPC服务列表</title>
	{{range .}}
	<hr>
	服务名 {{.Name}}
	<hr>
		<table>
		<th align=center>方法</th>
		<th align=center>调用次数</th>
		{{range $name, $mtype := .Method}}
			<tr>
			<td align=left font=fixed>func {{$name}}({{$mtype.ArgType}}, {{$mtype.ReplyType}}) error</td>
			<td align=center>{{$mtype.NumCalls}}</td>
			</tr>
		{{end}}
		</table>
	{{end}}
</body>
</html>`

var debug = template.Must(template.New("RPC debug").Parse(debugText))

type debugHTTP struct {
	*Server
}

type debugService struct {
	Name   string
	Method map[string]*methodType
}

func (s debugHTTP) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var services []debugService
	s.serviceMap.Range(func(namei, svci interface{}) bool {
		svc := svci.(*service)
		services = append(services, debugService{
			Name:   namei.(string),
			Method: svc.method,
		})
		return true
	})
	if err := debug.Execute(w, services); err != nil {
		_, _ = fmt.Fprintln(w, "rpc: error executing template:", err.Error())
	}
}
