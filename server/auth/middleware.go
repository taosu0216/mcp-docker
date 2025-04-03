package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// RequestLoggerMiddleware 创建一个HTTP中间件来记录请求详情，包括请求体
func RequestLoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 打印请求信息
		fmt.Printf("\n=== 请求详情开始 ===\n")
		fmt.Printf("方法: %s\n", r.Method)
		fmt.Printf("路径: %s\n", r.URL.Path)
		fmt.Printf("查询参数: %s\n", r.URL.RawQuery)
		fmt.Printf("远程地址: %s\n", r.RemoteAddr)
		fmt.Printf("用户代理: %s\n", r.UserAgent())

		// 打印请求头
		fmt.Printf("--- 请求头 ---\n")
		for name, values := range r.Header {
			for _, value := range values {
				fmt.Printf("%s: %s\n", name, value)
			}
		}

		// 打印请求体（只对POST和PUT请求）
		if r.Method == "POST" || r.Method == "PUT" {
			fmt.Printf("--- 请求体 ---\n")
			var bodyBytes []byte
			if r.Body != nil {
				bodyBytes, _ = io.ReadAll(r.Body)
				// 重新设置请求体，以便后续处理
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

				// 尝试解析JSON格式化输出
				var prettyJSON bytes.Buffer
				if json.Valid(bodyBytes) {
					err := json.Indent(&prettyJSON, bodyBytes, "", "  ")
					if err == nil {
						fmt.Printf("%s\n", prettyJSON.String())
					} else {
						fmt.Printf("%s\n", string(bodyBytes))
					}
				} else {
					fmt.Printf("%s\n", string(bodyBytes))
				}
			}
		}

		fmt.Printf("=== 请求详情结束 ===\n\n")

		// 继续处理请求
		next.ServeHTTP(w, r)
	})
}

// RequestBodyLoggingHandler 创建一个Handler包装器，记录请求体
func RequestBodyLoggingHandler(handler http.Handler) http.Handler {
	return RequestLoggerMiddleware(handler)
}
