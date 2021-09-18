package main

import (
	"context"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/device"
	"github.com/gin-gonic/gin"
)

type HandleMsg struct {
	Code int
	Msg  string
}

var (
	used      bool
	timestamp int64
	mu        sync.RWMutex
	ch        chan string
	chMsg     chan HandleMsg

	cookiePath = []string{"/root/jd_scripts/logs/cookies.list"}
)

func main() {
	ch = make(chan string)
	chMsg = make(chan HandleMsg)
	router := gin.Default()
	router.LoadHTMLGlob("html/*")
	router.GET("/sendsms", SendSms)
	router.GET("/submit", Submit)
	router.GET("/", func(c *gin.Context) {
		c.HTML(200, "index.html", nil)
	})
	router.Run(":12121")
}

func SendSms(c *gin.Context) {
	mobile := c.Query("mobile")
	code := 0
	msg := ""
	t := time.Now().UnixMilli()
	defer func() {
		if code == 0 {
			timestamp = t
		}
		c.JSON(200, gin.H{
			"code": code,
			"msg":  msg,
			"t":    t,
		})
	}()
	if mobile == "" {
		code = 1
		msg = "手机号不正确"
		return
	}
	mu.Lock()
	if used {
		code = 2
		msg = "前面有人正在登录，请等待"
		return
	}
	used = true
	mu.Unlock()
	go handle(mobile)
	res := <-chMsg
	code = res.Code
	msg = res.Msg
}

func Submit(c *gin.Context) {
	smsCode := c.Query("code")
	t, _ := strconv.ParseInt(c.Query("t"), 10, 64)
	code := 0
	msg := ""
	defer func() {
		c.JSON(200, gin.H{
			"code": code,
			"msg":  msg,
		})
	}()
	if smsCode == "" {
		code = 1
		msg = "验证码为空"
		return
	}

	log.Println(t, timestamp)
	if t != timestamp {
		code = 2
		msg = "请刷新页面重新获取"
		return
	}
	ch <- smsCode
	res := <-chMsg
	code = res.Code
	msg = res.Msg
}
func handle(mobile string) {
	defer func() {
		timestamp = 0
		mu.Lock()
		used = false
		mu.Unlock()
	}()
	var msg HandleMsg
	//本地测试用
	// options := []chromedp.ExecAllocatorOption{
	// 	chromedp.Flag("headless", false),
	// }
	// options = append(chromedp.DefaultExecAllocatorOptions[:], options...)
	// ctx, cancel := chromedp.NewExecAllocator(context.Background(), options...)
	// defer cancel()
	// ctx, cancel = chromedp.NewContext(ctx)
	// defer cancel()

	//docker
	ctx, cancel := chromedp.NewRemoteAllocator(context.Background(), "http://127.0.0.1:9222")
	defer cancel()
	ctx, cancel = chromedp.NewContext(ctx)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	var btnText string
	err := chromedp.Run(ctx, startJD(), sendKeys(".mobile", mobile),
		chromedp.Click("document.querySelector('.getMsg-btn')", chromedp.ByJSPath), chromedp.Sleep(time.Second*3),
		chromedp.Text(`.getMsg-btn`, &btnText, chromedp.NodeVisible))
	if err != nil {
		msg.Code = 999
		msg.Msg = err.Error()
		log.Println(err.Error())
		chMsg <- msg
		return
	}

	log.Println(btnText)
	if !strings.HasPrefix(btnText, "重新获取") {
		msg.Code = 3
		var dialogDes string
		chromedp.Run(ctx,
			chromedp.Text(`.dialog-des`, &dialogDes, chromedp.NodeVisible),
		)
		if dialogDes != "" {
			msg.Msg = dialogDes
		} else {
			msg.Msg = "未知错误"
			var bufs []byte
			chromedp.Run(ctx, chromedp.CaptureScreenshot(&bufs))
			if err := ioutil.WriteFile("error.fullScreenshot.png", bufs, 0644); err != nil {
				log.Println(err)
			}
		}
		chMsg <- msg
		return
	}
	var checked string
	var ok bool
	chromedp.Run(ctx, chromedp.AttributeValue(".policy_tip-checkbox", "checked", &checked, &ok, chromedp.ByQuery))
	log.Println("协议", checked)
	if checked != "true" {
		chromedp.Run(ctx, chromedp.Click("document.querySelector('.policy_tip-checkbox')", chromedp.ByJSPath))
	}
	chMsg <- msg

	for {
		select {
		case <-ctx.Done():
			log.Println("wait done")
			return
		// case <-time.After(time.Minute * 5):
		// 	log.Println("time out")
		// 	return
		case code := <-ch:
			var buf1 []byte
			chromedp.Run(ctx, chromedp.CaptureScreenshot(&buf1))
			if err := ioutil.WriteFile("fullScreenshot1.png", buf1, 0644); err != nil {
				log.Fatal(err)
			}
			err := chromedp.Run(ctx, sendKeys("#authcode", code), chromedp.Click("document.querySelector('.btn')", chromedp.ByJSPath), chromedp.Sleep(time.Second*2))
			if err != nil {
				msg.Code = 999
				msg.Msg = err.Error()
				log.Println(err.Error())
				chMsg <- msg
				return
			}
			var url string
			err = chromedp.Run(ctx, chromedp.Location(&url))
			log.Println(url, err)
			if strings.HasPrefix(url, "https://m.jd.com/") {
				var pt_pin, pt_key string
				chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
					cookies, err := network.GetAllCookies().Do(ctx)
					if err != nil {
						return err
					}
					for _, cookie := range cookies {
						if cookie.Name == "pt_pin" {
							pt_pin = cookie.Value
						} else if cookie.Name == "pt_key" {
							pt_key = cookie.Value
						}
						// log.Printf("chrome cookie %d: %+v", i, cookie)
					}
					return nil
				}))
				cookie := "pt_key=" + pt_key + ";pt_pin=" + pt_pin + ";"
				log.Println(cookie)
				updateCookie(pt_pin, cookie)
				msg.Msg = cookie
				//todo 更新容器内cookie
			} else {
				//登录失败
				//todo 获取错误信息
				//验证码错误继续提交
				var bufs []byte
				chromedp.Run(ctx, chromedp.CaptureScreenshot(&bufs))
				if err := ioutil.WriteFile("error.login.fullScreenshot.png", bufs, 0644); err != nil {
					log.Println(err)
				}
				msg.Code = 1
				msg.Msg = "登录失败"
			}
			chMsg <- msg
			return
		}
	}
}

func startJD() chromedp.Tasks {
	return chromedp.Tasks{
		chromedp.Emulate(device.IPhone11),
		chromedp.Navigate(`https://plogin.m.jd.com/login/login?appid=300&returnurl=https%3A%2F%2Fwq.jd.com%2Fpassport%2FLoginRedirect%3Fstate%3D2040606433%26returnurl%3Dhttps%253A%252F%252Fhome.m.jd.com%252FmyJd%252Fnewhome.action%253Fsceneval%253D2%2526ufc%253D%2526&source=wq_passport`),
		chromedp.WaitVisible(`#app`, chromedp.ByID),
		chromedp.Click("document.querySelector('.policy_tip-checkbox')", chromedp.ByJSPath),
	}
}

func sendKeys(sel, keys string) chromedp.Tasks {
	var tasks chromedp.Tasks
	tasks = append(tasks, chromedp.Focus(sel), chromedp.SetValue(sel, "", chromedp.NodeVisible), chromedp.Sleep(time.Second))
	for _, s := range keys {
		tasks = append(tasks, chromedp.SendKeys(sel, string(s), chromedp.NodeVisible), chromedp.Sleep(time.Millisecond*300))
	}
	tasks = append(tasks, chromedp.Sleep(time.Second))
	return tasks
}

func updateCookie(pt_pin, cookie string) {
	for _, path := range cookiePath {
		b, err := readFile(path)
		if err != nil {
			log.Println(err)
			continue
		}
		allCookie := string(b)
		if strings.Contains(allCookie, "pt_pin="+pt_pin+";") {
			cookies := strings.Split(allCookie, "\n")
			for i, c := range cookies {
				if strings.Contains(c, "pt_pin="+pt_pin+";") {
					cookies[i] = cookie
					if i > 0 && strings.Contains(cookies[i-1], "上次更新") {
						cookies[i-1] = "## " + pt_pin + " 上次更新:" + time.Now().Format("2006-01-02 15:04:05")
					} else {
						cookies = append(cookies[:i], append([]string{"## " + pt_pin + " 上次更新:" + time.Now().Format("2006-01-02 15:04:05")}, cookies[i:]...)...)
					}
					break
				}
			}
			allCookie = strings.Join(cookies, "\n")

		} else {
			allCookie += "\n" + "## " + pt_pin + " 上次更新:" + time.Now().Format("2006-01-02 15:04:05") + "\n" + cookie
		}
		if err := writeFile(path, []byte(allCookie)); err != nil {
			log.Println(err)
		}
	}
}

func readFile(filePth string) ([]byte, error) {
	f, err := os.Open(filePth)
	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(f)
}

func writeFile(filePth string, b []byte) error {
	f, err := os.OpenFile(filePth, os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(b)
	return err
}
