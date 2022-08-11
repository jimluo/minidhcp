package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
)

// RestClient: 从管控中心和其他地方获取
type (
	RestClient struct {
		url string //= "https://ip:port/v1/auth/"
	}
)

const (
	urlHost = "http://%s/"
	userUrl = "/auth/user/listall"
)

func (m *RestClient) requestCenter(isPost bool, cmd string, param []byte, result interface{}) error {
	var resp *http.Response
	var err error
	if isPost {
		resp, err = http.Post(m.url+cmd, "Content-type: application/json", bytes.NewBuffer(param))
	} else {
		resp, err = http.Get(m.url + cmd)
	}
	if err != nil {
		// log.Error(cmd, log.String("err", err.Error()))
		return err
	}
	defer resp.Body.Close()
	// log.Info("response Status Headers", resp.Status, resp.Header)

	body, _ := ioutil.ReadAll(resp.Body)
	if !bytes.ContainsAny(body, "QS000000") {
		err = errors.New("Response code error" + string(body))
	} else {
		err = json.Unmarshal(body, &result)
	}
	if err != nil {
		// log.Error("Response body", log.String("err", err.Error()))
	}
	// r := (*response)(unsafe.Pointer(&result)) // Linux go 报错
	// codePtr := reflect.ValueOf(result)
	// code := reflect.Indirect(codePtr).FieldByName("Code")
	// if code.String() != "QS000000" {
	// }

	return err
}

// 启动时，先从控制中心一次性获取加载配置
func InitByRestClient(ipport string) *RestClient {
	client := RestClient{fmt.Sprintf(urlHost, ipport)}
	return &client
}
