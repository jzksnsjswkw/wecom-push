package wecom

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"sync"
)

// cspell: disable
type accessResp struct {
	Errcode     int    `json:"errcode"`
	Errmsg      string `json:"errmsg"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type wecom struct {
	corpid                string
	corpsecret            string
	accessToken           string
	pushLock              *sync.Mutex
	initLock              *sync.Mutex
	isFirstAccessTokenErr bool
}

func New(corpid, corpsecret string) *wecom {
	return &wecom{
		corpid:                corpid,
		corpsecret:            corpsecret,
		pushLock:              &sync.Mutex{},
		initLock:              &sync.Mutex{},
		isFirstAccessTokenErr: true,
	}
}

func (w *wecom) getAccessToken() error {
	reqUrl := "https://qyapi.weixin.qq.com/cgi-bin/gettoken"
	d := url.Values{
		"corpid":     {w.corpid},
		"corpsecret": {w.corpsecret},
	}
	reqUrl += "?" + d.Encode()

	r, err := http.NewRequest(http.MethodPost, reqUrl, nil)
	if err != nil {
		return err
	}
	r.Header.Add("accept", "application/json")
	r2, err := http.DefaultClient.Do(r)
	if err != nil {
		return err
	}
	defer r2.Body.Close()

	b, err := io.ReadAll(r2.Body)
	if err != nil {
		return err
	}
	a := &accessResp{}
	if err := json.Unmarshal(b, a); err != nil {
		return err
	}
	if a.Errcode != 0 {
		return errors.New(a.Errmsg)
	}

	w.accessToken = a.AccessToken
	return nil
}

func (w *wecom) send(getResp func() ([]byte, error)) ([]byte, error) {
	err := func() error {
		w.initLock.Lock()
		defer w.initLock.Unlock()
		if w.accessToken == "" {
			err := w.getAccessToken()
			if err != nil {
				return err
			}
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}

	resp, err := getResp()
	if err != nil {
		return nil, err
	}
	r := struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}{}
	if err := json.Unmarshal(resp, &r); err != nil {
		return nil, err
	}

	w.pushLock.Lock()
	if r.ErrCode == 0 {
		w.pushLock.Unlock()
	} else if r.ErrCode == 42001 || r.ErrCode == 40014 || r.ErrCode == 41001 {
		if w.isFirstAccessTokenErr {
			switch r.ErrCode {
			case 42001:
				fmt.Println("access_token过期")
			case 40014:
				fmt.Println("access_token无效")
			case 41001:
				fmt.Println("access_token错误")
			}
			w.isFirstAccessTokenErr = false
			err := w.getAccessToken()
			w.pushLock.Unlock()
			if err != nil {
				return nil, err
			}
			resp, err = w.send(getResp)
			if err != nil {
				return nil, err
			}
		} else {
			w.pushLock.Unlock()
			w.isFirstAccessTokenErr = true
			resp, err = w.send(getResp)
			if err != nil {
				return nil, err
			}
		}
	} else {
		w.pushLock.Unlock()
		return nil, errors.New(r.ErrMsg)
	}

	return resp, nil
}

type TextInfo struct {
	Touser  string
	AgentID int
	Content string
}

func (w *wecom) Text(t *TextInfo) error {
	buf := func() ([]byte, error) {
		url := "https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=" + w.accessToken
		d := map[string]any{
			"touser":  t.Touser,
			"msgtype": "text",
			"agentid": t.AgentID,
			"text": map[string]string{
				"content": t.Content,
			},
			"safe": "0",
		}
		b, err := json.Marshal(d)
		if err != nil {
			return nil, err
		}
		r, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		r.Header.Add("content-type", "application/json")
		r.Header.Add("accept", "application/json")
		r2, err := http.DefaultClient.Do(r)
		if err != nil {
			return nil, err
		}
		defer r2.Body.Close()

		b2, err := io.ReadAll(r2.Body)
		if err != nil {
			return nil, err
		}
		return b2, nil
	}
	if _, err := w.send(buf); err != nil {
		return err
	}
	return nil
}

type Filetype string

const (
	IMAGE Filetype = "image"
	VOICE Filetype = "voice"
	VIDEO Filetype = "video"
	FILE  Filetype = "file"
)

func (w *wecom) getMediaID(content []byte, filetype Filetype, filename string) (string, error) {
	buf := func() ([]byte, error) {
		url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/media/upload?access_token=%v&type=%v", w.accessToken, filetype)
		b := &bytes.Buffer{}
		writer := multipart.NewWriter(b)
		part, err := writer.CreateFormFile("media", filename)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(part, bytes.NewReader(content)); err != nil {
			return nil, err
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
		r, err := http.NewRequest(http.MethodPost, url, b)
		if err != nil {
			return nil, err
		}
		r.Header.Add("content-type", writer.FormDataContentType())
		r.Header.Add("accept", "application/json")
		r2, err := http.DefaultClient.Do(r)
		if err != nil {
			return nil, err
		}
		defer r2.Body.Close()

		b2, err := io.ReadAll(r2.Body)
		if err != nil {
			return nil, err
		}

		return b2, nil
	}

	b, err := w.send(buf)
	if err != nil {
		return "", err
	}

	m := map[string]any{}
	if err := json.Unmarshal(b, &m); err != nil {
		return "", err
	}
	return m["media_id"].(string), nil
}

type FileInfo struct {
	Touser   string
	AgentID  int
	Content  []byte
	Filetype Filetype
	Filename string

	// 仅VIDEO有效
	Title string
	// 仅VIDEO有效
	Description string
}

func (w *wecom) File(f *FileInfo) error {
	m, err := w.getMediaID(f.Content, f.Filetype, f.Filename)
	if err != nil {
		return err
	}

	buf := func() ([]byte, error) {
		url := "https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=" + w.accessToken
		m := map[string]any{
			"touser":  f.Touser,
			"msgtype": f.Filetype,
			"agentid": f.AgentID,
			string(f.Filetype): map[string]string{
				"media_id":    m,
				"title":       f.Title,
				"description": f.Description,
			},
			"safe": 0,
		}
		b, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		r, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		r.Header.Add("content-type", "application/json")
		r.Header.Add("accept", "application/json")

		r2, err := http.DefaultClient.Do(r)
		if err != nil {
			return nil, err
		}
		defer r2.Body.Close()

		body, err := io.ReadAll(r2.Body)
		if err != nil {
			return nil, err
		}
		return body, nil
	}

	if _, err := w.send(buf); err != nil {
		return err
	}
	return nil
}
