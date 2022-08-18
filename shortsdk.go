package main

import (
	"regexp"
	"strconv"
	"strings"
)

type LinkInfo struct {
	URL          string
	URLID        uint32
	IP           string
	HitCounter   uint
	Paid         bool
	Resolved     bool
	ReturnStatus int
	Title        string
	PreviewImage string
	PreviewText  string
}

var linksRoot = "http://127.0.0.1:12345/"
var linkRE = regexp.MustCompile(`(https?:\/\/\S+)`)
var shortLinkRE = regexp.MustCompile("(" + linksRoot + `\d+)`)

func linkReplace(src string) string {
	b := []byte(src)
	b = linkRE.ReplaceAllFunc(b, linkShortInplace)
	return (string(b))
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

func getItemPreview(item *LinkInfo) string {
	if item.PreviewImage == "" {
		return ""
	}
	return ("<img style=display:block src='" + item.PreviewImage + "' border=0>")

}

func shortLinkWidget(uid uint32) string {
	link := shortLinkURL(uid)
	return "<span title=status:" +
		strconv.Itoa(int(linkhash[uid].ReturnStatus)) +
		" style='color:cyan;border:1px dashed blue;border-radius:5px;background:green'><a style=color:cyan href=" + link + ">" +
		linkhash[uid].Title +
		"</a> (" +
		strconv.Itoa(int(linkhash[uid].HitCounter)) +
		")" + getItemPreview(linkhash[uid]) + "</span>"
}
