package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"net/http"
	"text/template"
)

type ThreadInfo struct {
	thread []PostInfo
}

type PostInfo struct {
	Name         string
	Email        string
	Message      string
	IP           string
	BlessedByAbu bool
}

var templatePost = template.Must(template.New("post").Funcs(
	template.FuncMap{"blessed": blessed, "linkWidget": inflateLinkWidgets},
).Parse(
	`
	<hr>
	<div style=border:1px><a href=mailto:{{.Email}}>{{.Name}}</a><br>{{.Message|linkWidget}}{{.BlessedByAbu|blessed}}</div>
	`))
var templateThread = template.Must(template.New("thread").Parse(
	`
		<ul>
		{{range .Thread}}
		<li><a href=/board/{{.ID}}>{{.ID}}</a>
		{{end}}
		</ul>
		`))

func blessed(arg bool) string {
	if arg {
		return "<br><br>Пост благославлен Абу"
	}
	return ""
}

func (p PostInfo) String() string {
	buf := &bytes.Buffer{}
	templatePost.Execute(buf, p)
	return buf.String()
}
func (t ThreadInfo) String() string {
	buf := &bytes.Buffer{}
	templateThread.Execute(buf, t)
	return buf.String()
}

var boardhash = []PostInfo{}

func boardHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method == "POST" {
		msg := r.PostFormValue("message")
		msg = linkReplace(msg)

		tmp := PostInfo{
			Name:         "Anonymous",
			Email:        "",
			Message:      msg,
			IP:           r.RemoteAddr,
			BlessedByAbu: rand.Int()%10 > 8,
		}
		boardhash = append(boardhash, tmp)
		w.Header().Add("Location", "/board/")
		w.WriteHeader(http.StatusSeeOther)
		return
	}

	w.Header().Add("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "%v", boardhash)
	fmt.Fprintf(w, `
<form action=/board/ method=post>
<textarea cols=50 rows=5 name=message></textarea><br>
<input type=submit>
</form>
`)

}
