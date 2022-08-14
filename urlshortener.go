package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
)

var linksRoot = "http://127.0.0.1:12345/"
var linkRE = regexp.MustCompile(`(https?:\/\/\S+)`)
var shortLinkRE = regexp.MustCompile("(" + linksRoot + `\d+)`)
var notRealError = errors.New("shit")

var header = `<html>
<head><title>my cool website</title>
</head><body bgcolor=seashell>
`

var formochka = `
<form method=post action="/short">
Please input your LONG URL to short this:<br>
<input type=text name=url value=https://>
<input type=submit>
</form>
`
var urlResolverChan = make(chan uint32)

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

type UserInfo struct {
	Age       int
	HasPenis  bool
	HasVagina bool
	Language  string
}

type LinkInfo struct {
	URL        string
	URLID      uint32
	IP         string
	HitCounter uint
	Paid       bool
	Resolved   bool
}

var linkhash = make(map[uint32]*LinkInfo)

func linkReplace(src string) string {
	b := []byte(src)
	b = linkRE.ReplaceAllFunc(b, linkShortInplace)
	return (string(b))
}

func formHandler(w http.ResponseWriter, r *http.Request) {
	uri := string(r.URL.Path)
	uri_parts := strings.Split(uri, "/")
	uid64, _ := strconv.ParseUint(uri_parts[1], 10, 32)
	uid := uint32(uid64)

	item := linkhash[uid]
	if item != nil {

		item.HitCounter++

		if item.Paid {
			w.Header().Add("Location", item.URL)
			w.WriteHeader(http.StatusSeeOther)
			return
		}
	}

	w.Header().Add("Content-Type", "text/html")
	fmt.Fprintf(w, header)
	fmt.Fprintf(w, formochka)
	if item != nil {
		fmt.Fprintf(w, `Your url: %s<br>
	Hits: %d`, item.URL, item.HitCounter)
	}
}
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

func linkShort(url string, ip string) uint32 {

	if shortLinkRE.Match([]byte(url)) {
		urlParts := strings.Split(url, "/")
		uid64, _ := strconv.ParseUint(urlParts[3], 10, 32)
		uid := uint32(uid64)
		_, exists := linkhash[uid]
		if exists {
			return (uid)
		}

	}

	var uid uint32 = rand.Uint32()
	tmpInfo := LinkInfo{
		URL:        url,
		IP:         ip,
		URLID:      uid,
		HitCounter: 0,
		Resolved:   false,
	}
	linkhash[uid] = &tmpInfo
	dump, _ := json.Marshal(&linkhash)
	f, _ := os.Create("db.json")
	f.Write(dump)
	f.Close()
	urlResolverChan <- uid
	return (uid)
}

func urlResolverWorker() {
	var uid uint32
	var currentURL string
	for {
		uid = <-urlResolverChan
		item, ok := linkhash[uid]
		if !ok || item.Resolved {
			continue
		}
		currentURL = item.URL

		tr := &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}

		c := &http.Client{
			CheckRedirect: getNil,
			Transport:     tr,
		}

		for try := 20; try > 0; try-- {
			fmt.Printf("%d: Resolving %s...\n", try, currentURL)
			req, _ := http.NewRequest("GET", currentURL, nil)
			resp, weberr := c.Do(req)
			//fmt.Printf("got %v %v\n", resp, weberr)

			if weberr != nil && weberr != notRealError {

				fmt.Printf("Can't load page: %s\n", weberr)
				break

			}
			loc := resp.Header.Get("Location")
			if loc != "" {
				currentURL = loc
				continue
			}
			break
		}
		item.URL = currentURL

	}
}

// http://shorturl.at/aclrX
// http://alii.pub/6ekjp8
func getNil(_ *http.Request, _ []*http.Request) error {
	return notRealError
}

func linkShortInplace(src []byte) []byte {
	uid := linkShort(string(src), "")
	return []byte(shortLinkURL(uid))
}

func shortLinkURL(uid uint32) string {
	return linksRoot + strconv.FormatUint(uint64(uid), 10)
}

func inflateLinkWidgets(msg string) string {
	b := []byte(msg)
	b = shortLinkRE.ReplaceAllFunc(b, inflateLinkWidgetsInplace)
	return (string(b))
}

func inflateLinkWidgetsInplace(in []byte) []byte {
	str := string(in)
	strParts := strings.Split(str, "/")
	uid64, _ := strconv.ParseUint(strParts[3], 10, 32)
	uid := uint32(uid64)
	return ([]byte(shortLinkWidget(uid)))
}

func shortLinkWidget(uid uint32) string {
	link := shortLinkURL(uid)
	return "<a href=" + link + ">" + link + "</a> (" + strconv.Itoa(int(linkhash[uid].HitCounter)) + ")"
}

func shortHandler(w http.ResponseWriter, r *http.Request) {
	url := r.PostFormValue("url")
	uid := linkShort(url, r.RemoteAddr)
	w.Header().Add("Location", shortLinkURL(uid))
	w.WriteHeader(http.StatusTemporaryRedirect)
}

func main() {

	rand.Seed(time.Now().UnixMilli())

	dump, err := os.ReadFile("db.json")
	if err == nil {
		json.Unmarshal(dump, &linkhash)
	}

	go urlResolverWorker()

	http.HandleFunc("/board/", boardHandler)
	http.HandleFunc("/short", shortHandler)
	http.HandleFunc("/", formHandler)
	http.ListenAndServe("127.0.0.1:12345", nil)

}
