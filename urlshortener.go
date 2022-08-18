package main

import (
	"bytes"
	"crypto/md5"
	"crypto/tls"
	b64 "encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var paramRE = regexp.MustCompile("([?&])[^?&]+$")
var noScriptRE = regexp.MustCompile(`(?is)<script[^>]*>.+?</script>|<meta[^>]+>`)

type LinkTransform struct {
	SrcRule string
	DstRule string
	Proc    func(item *LinkInfo)
}

type Site struct {
	Example string // not used, or for test:  Your url: https://nl.aliexpress.com/item/1005004131106285.html
	Host    string // aliexpress.com, not www.aliexpress.com
	Rules   []LinkTransform
	Harmful bool
}

var sitehash = make(map[string]*Site)
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

type UserInfo struct {
	Age       int
	HasPenis  bool
	HasVagina bool
	Language  string
}

var linkhash = make(map[uint32]*LinkInfo)

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

func linkShort(url string, ip string) uint32 {

	// check link to our service
	if shortLinkRE.Match([]byte(url)) {
		urlParts := strings.Split(url, "/")
		uid64, _ := strconv.ParseUint(urlParts[3], 10, 32)
		uid := uint32(uid64)
		_, exists := linkhash[uid]
		if exists {
			return (uid)
		}

	}

	if !(strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://")) {
		return 0
	}

	var uid uint32 = rand.Uint32()
	tmpInfo := LinkInfo{
		URL:        url,
		IP:         ip,
		URLID:      uid,
		HitCounter: 0,
		Resolved:   false,
		Title:      url,
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

	var respBody bytes.Buffer

	for {
		uid = <-urlResolverChan
		item, ok := linkhash[uid]

		if !ok || item.Resolved {
			continue
		}
		currentURL = item.URL
		fmt.Printf("Processing uid:%d, link:%s\n", uid, currentURL)

		tr := &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}

		c := &http.Client{
			CheckRedirect: getNil,
			Transport:     tr,
		}

		// follow for redirects

		for try := 20; try > 0; try-- {

			utmRE := regexp.MustCompile("([?&#])(utm_[^&=]+=[^&]+|(?si)phpsessid=[a-z0-9]{32})")
			currentURL = utmRE.ReplaceAllString(currentURL, "$1")

			fmt.Printf("%d: Resolving %s...\n", try, currentURL)
			req, _ := http.NewRequest("GET", currentURL, nil)
			resp, weberr := c.Do(req)
			//fmt.Printf("got %v %v\n", resp, weberr)

			if resp == nil {

				fmt.Printf("Can't load page: %s\n", weberr)
				break

			}
			item.ReturnStatus = resp.StatusCode
			respBody.Reset()
			io.Copy(&respBody, resp.Body)
			resp.Body.Close()

			loc := resp.Header.Get("Location")
			if loc != "" {
				currentURL = loc
				continue
			}
			break
		}
		orig := respBody.String()
		orig = noScriptRE.ReplaceAllString(orig, "")

		item.URL = currentURL
		original := md5.Sum([]byte(orig))

		// remove all shit in URL (in query string)
		parsedURL, _ := url.Parse(currentURL)
		//linkAnchor := parsedURL.RawFragment
		linkQuery := parsedURL.RawQuery

		for try := 20; linkQuery != "" && try >= 0; try-- {
			fmt.Printf("cutting %s\n", linkQuery)
			linkQuery2 := paramRE.ReplaceAllString(linkQuery, "")
			if linkQuery == linkQuery2 {
				linkQuery2 = ""
			}
			linkQuery = linkQuery2

			parsedURL.RawQuery = linkQuery
			req, _ := http.NewRequest("GET", parsedURL.String(), nil)
			resp, _ := c.Do(req)
			if resp != nil {
				respBody.Reset()
				io.Copy(&respBody, resp.Body)
				resp.Body.Close()
				item.ReturnStatus = resp.StatusCode
				newRespStr := respBody.String()
				newRespStr = noScriptRE.ReplaceAllString(newRespStr, "")
				//os.WriteFile("page-"+strconv.Itoa(try)+".txt", []byte(newRespStr), 0)

				newhash := md5.Sum([]byte(newRespStr))
				fmt.Printf("original: %v, newhash: %v\n",
					hex.EncodeToString(original[:]),
					hex.EncodeToString(newhash[:]))
				if newhash == original {
					currentURL = parsedURL.String()
					item.URL = currentURL
				} else {
					//					break
				}
			}

		}

		// extract site and do site-predefined transformations
		stripPort := regexp.MustCompile(`:\d+`)
		defer respBody.Reset()

		site := parsedURL.Host
		site = stripPort.ReplaceAllString(site, "")
		domainParts := strings.Split(site, ".")
		if len(domainParts) > 1 {
			site = strings.Join(domainParts[len(domainParts)-2:], ".")
		}
		fmt.Printf("got site: %s\n", site)

		siteInfo, knownSite := sitehash[site]
		if knownSite {
			rules := siteInfo.Rules
			for _, rule := range rules {
				if rule.SrcRule != "" {
					tmpRE := regexp.MustCompile(rule.SrcRule)
					currentURL = tmpRE.ReplaceAllString(currentURL, rule.DstRule)
					item.URL = currentURL
				}
				if rule.Proc != nil {
					rule.Proc(item)
				}
			}

		}

		item.URL = currentURL

		// Extract title and other info
		titleRE := regexp.MustCompile(`(?is)<title[^>]*>([^<]+)`)
		item.Title = currentURL
		titles := titleRE.FindStringSubmatch(respBody.String())
		if titles != nil {
			fmt.Printf("possible title: %v\n", titles[1])
			item.Title = titles[1]
		}

		// adf.ly
		adf := adfly_decode(respBody.String())
		fmt.Printf("adf: %s\n", adf)
		if adf != "" {
			item.URL = adf
			urlResolverChan <- uid

		}

	}
}

// http://shorturl.at/aclrX
// http://alii.pub/6ekjp8
// http://bit.ly/1YWH7ea
// http://lyksoomu.com/6bPK
// https://www.youtube.com/watch?v=yyUHQIec83I
// https://youtu.be/yyUHQIec83I
func getNil(_ *http.Request, _ []*http.Request) error {
	return notRealError
}

func adfly_decode(body string) string {
	ysmmRE := regexp.MustCompile(`var ysmm = '([^';]+)'`)
	ysmmRet := ysmmRE.FindStringSubmatch(body)
	if ysmmRet == nil {
		return ("")
	}
	return adfly_ysmm_decode(ysmmRet[1])
}

func adfly_ysmm_decode(ysmm string) string {

	var C string = ""
	var h string = ""

	for s, srune := range ysmm {
		if s%2 == 0 {
			C += string(srune)
		} else {
			h = string(srune) + h
		}
	}

	a := strings.Split(C+h, "")

	for b := 0; b < len(a); b++ {
		rr := []rune(a[b])
		if unicode.IsDigit(rr[0]) {
			for c := b + 1; c < len(a); c++ {
				rr2 := []rune(a[c])
				if unicode.IsDigit(rr2[0]) {
					d1, _ := strconv.ParseInt(a[b], 10, 64)
					d2, _ := strconv.ParseInt(a[c], 10, 64)
					d := d1 ^ d2
					if d < 10 {
						a[b] = strconv.Itoa(int(d))
					}
					b = c
					c = len(a)
				}
			}
		}
	}
	linkBytes, _ := b64.StdEncoding.DecodeString(strings.Join(a, ""))

	return string(linkBytes[16 : len(linkBytes)-16])

}

func youtube_preview(item *LinkInfo) {
	tubeRE := regexp.MustCompile(`(watch\?v=|be/)([a-zA-Z0-9_-]{11})`)
	tubeRet := tubeRE.FindStringSubmatch(item.URL)
	if tubeRet != nil {
		item.PreviewImage = "https://img.youtube.com/vi/" + tubeRet[2] + "/mqdefault.jpg"
	}

}

func linkShortInplace(src []byte) []byte {
	uid := linkShort(string(src), "")
	return []byte(shortLinkURL(uid))
}

func shortLinkURL(uid uint32) string {
	return linksRoot + strconv.FormatUint(uint64(uid), 10)
}

func shortHandler(w http.ResponseWriter, r *http.Request) {
	url := r.PostFormValue("url")
	uid := linkShort(url, r.RemoteAddr)
	w.Header().Add("Location", shortLinkURL(uid))
	w.WriteHeader(http.StatusTemporaryRedirect)
}

func main() {

	sitehash["aliexpress.com"] = &Site{
		"https://nl.aliexpress.com/item/1005004131106285.html",
		"aliexpress.com",
		[]LinkTransform{
			{`[a-z]+\.aliexpress.[a-z]+`, "www.aliexpress.com", nil},
		},
		false,
	}

	sitehash["aliexpress.ru"] = &Site{
		"https://aliexpress.ru/item/1005004131106285.html",
		"aliexpress.ru",
		[]LinkTransform{
			{`[a-z]+\.aliexpress.[a-z]+`, "www.aliexpress.com", nil},
		},
		false,
	}
	sitehash["youtube.com"] = &Site{
		"https://www.youtube.com/watch?v=yyUHQIec83I",
		"youtube.com",
		[]LinkTransform{
			{`feature=youtu.be`, "", nil},
			{Proc: youtube_preview},
		},
		false,
	}

	rand.Seed(time.Now().UnixMilli())

	dump, err := os.ReadFile("db.json")
	if err == nil {
		json.Unmarshal(dump, &linkhash)
	}

	go urlResolverWorker()
	go urlResolverWorker()
	go urlResolverWorker()
	go urlResolverWorker()
	go urlResolverWorker()

	http.HandleFunc("/board/", boardHandler)
	http.HandleFunc("/short", shortHandler)
	http.HandleFunc("/", formHandler)
	http.ListenAndServe("127.0.0.1:12345", nil)

}
