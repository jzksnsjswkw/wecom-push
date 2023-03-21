# wecom-push

企业微信推送 golang sdk

自动管理 access_token  
线程安全

## Example

```Go
package main

import (
	"os"

	"github.com/jzksnsjswkw/wecom-push"
)

func main() {
	corpid := "xxxxx"
	corpsecret := "xxxxxx"
	w := wecom.New(corpid, corpsecret)

	err := w.Text(&wecom.TextInfo{
		Touser:  "Pony",
		AgentID: 1000002,
		Content: "test",
	})
	if err != nil {
		panic(err)
	}

	b, err := os.ReadFile("./test.txt")
	if err != nil {
		panic(err)
	}
	err = w.File(&wecom.FileInfo{
		Touser:   "Pony",
		AgentID:  1000002,
		Content:  b,
		Filetype: wecom.FILE,
		Filename: "test.txt",
	})
	if err != nil {
		panic(err)
	}
}
```
