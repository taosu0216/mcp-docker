package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// 打印详细的请求信息，包括请求体
func PrintRequestDebugWithBody(r *http.Request, prefix string) {
	if !DEBUG {
		return
	}

	fmt.Printf("\n%s === 请求详情开始 ===\n", prefix)
	fmt.Printf("%s 方法: %s\n", prefix, r.Method)
	fmt.Printf("%s 路径: %s\n", prefix, r.URL.Path)
	fmt.Printf("%s 查询: %s\n", prefix, r.URL.RawQuery)
	fmt.Printf("%s 远程地址: %s\n", prefix, r.RemoteAddr)
	fmt.Printf("%s User-Agent: %s\n", prefix, r.UserAgent())

	fmt.Printf("%s --- 请求头 ---\n", prefix)
	for name, values := range r.Header {
		fmt.Printf("%s %s: %s\n", prefix, name, strings.Join(values, ", "))
	}

	fmt.Printf("%s --- 查询参数 ---\n", prefix)
	for key, values := range r.URL.Query() {
		fmt.Printf("%s %s: %s\n", prefix, key, strings.Join(values, ", "))
	}

	// 打印请求体
	if r.Method == "POST" || r.Method == "PUT" {
		fmt.Printf("%s --- 请求体 ---\n", prefix)

		// 由于请求体只能读取一次，所以需要先读取然后重置
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			fmt.Printf("%s 读取请求体失败: %v\n", prefix, err)
		} else {
			// 重置请求体，以便后续处理可以再次读取
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

			// 尝试解析JSON
			var jsonBody interface{}
			if err := json.Unmarshal(bodyBytes, &jsonBody); err == nil {
				// 美化输出JSON
				prettyJSON, err := json.MarshalIndent(jsonBody, prefix+" ", "  ")
				if err == nil {
					fmt.Printf("%s %s\n", prefix, string(prettyJSON))
				} else {
					// 如果无法美化，则原样输出
					fmt.Printf("%s %s\n", prefix, string(bodyBytes))
				}
			} else {
				// 如果不是JSON，则原样输出
				fmt.Printf("%s %s\n", prefix, string(bodyBytes))
			}
		}
	}

	fmt.Printf("%s === 请求详情结束 ===\n\n", prefix)
}
