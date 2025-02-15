package query

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/Karmenzind/kd/config"
	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/internal/model"
	"github.com/Karmenzind/kd/pkg"
	"github.com/anaskhan96/soup"
	"go.uber.org/zap"
)

var ydCliLegacy = &http.Client{}
var ydCli = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}

func requestYoudao(r *model.Result) (body []byte, err error) {
	var req *http.Request
	var url string
	var cli *http.Client
    useNewApi := false
	q := strings.ReplaceAll(r.Query, " ", "%20")
	if useNewApi {
		cli = ydCli
		url = fmt.Sprintf("https://dict.youdao.com/result?word=%s&lang=en", q)
	} else {
		cli = ydCliLegacy
		url = fmt.Sprintf("http://dict.youdao.com/w/%s/#keyfrom=dict2.top", q)
		// url = fmt.Sprintf("http://dict.youdao.com/search?q=%s", q)
	}
	req, err = http.NewRequest("GET", url, nil)
	if err != nil {
		zap.S().Errorf("Failed to create request: %s", err)
		return
	}
	if r.IsLongText {
		req.Header.Set("Upgrade-Insecure-Requests", "1")
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Host", "dict.youdao.com")
	req.Header.Set("User-Agent", pkg.GetRandomUA())

	resp, err := cli.Do(req)
	if err != nil {
		zap.S().Infof("[http] Failed to do request: %s", err)
		return
	}

	defer resp.Body.Close()
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		zap.S().Infof("[http] Failed to read response: %s", err)
		return
	}
	zap.S().Debugf("[http-get] query '%s' Resp len: %d Status: %v", url, len(body), resp.Status)
	if resp.StatusCode != 200 {
		zap.S().Debugf("[http-get] detail: header %+v", url, len(body), resp.Header)
	}
	if config.Cfg.Debug {
		errW := os.WriteFile(fmt.Sprintf("/home/k/Workspace/kd/data/%s.html", r.Query), (body), 0666)
		if errW != nil {
			zap.S().Warnf("Failed to write file '%s': %s", r.Query, errW)
		}
	}
	return
}

func parseHtml(resp string, r *model.Result) (err error) {
	return
}

// "star star5"
func parseCollinsStar(v string) (star int) {
	if strings.HasPrefix(v, "star star") && len(v) == 10 {
		intChar := v[9]
		star, _ = strconv.Atoi(string(intChar))
	}
	return
}

// return html
func FetchOnline(r *model.Result) (err error) {
	body, err := requestYoudao(r)
	if err != nil {
		zap.S().Infof("[http-youdao] Failed to request: %s", err)
		return
	}

	doc := soup.HTMLParse(string(body))
	yr := YdResult{r, &doc}

	if r.IsLongText {
		yr.parseMachineTrans()
		if r.MachineTrans != "" {
			r.Found = true
		}
		return
	}

	yr.parseParaphrase()
	if yr.isNotFound() {
		go cache.AppendNotFound(r.Query)
		return
	}

	// XXX (k): <2024-01-02> long text?
	yr.parseKeyword()
	yr.parsePronounce()
	yr.parseCollins()
	yr.parseExamples()

	r.Found = true
	go cache.UpdateQueryCache(r)
	return
}

func init() {
	ydCli = pkg.CreateHTTPClient(5)
}
