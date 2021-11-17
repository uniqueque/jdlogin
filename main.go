package main

import (
	"context"
	"fmt"
	"io/ioutil"
	pb "jdlogin/proto"
	"log"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/device"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
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

var (
	conn       *grpc.ClientConn
	grpcClient pb.OpenCVClient
)

func main() {
	var err error

	conn, err = grpc.Dial("127.0.0.1:12221", grpc.WithInsecure())

	if err != nil {
		log.Fatal("连接 gPRC 服务失败,", err)
	}
	defer conn.Close()
	// 创建 gRPC 客户端
	grpcClient = pb.NewOpenCVClient(conn)

	rand.Seed(time.Now().UnixNano())
	ch = make(chan string)
	chMsg = make(chan HandleMsg)
	router := gin.Default()
	router.LoadHTMLGlob("html/*")
	router.GET("/sendsms", SendSms)
	router.GET("/submit", Submit)
	router.GET("/showimage", ShowImage)
	router.GET("/", func(c *gin.Context) {
		c.HTML(200, "index.html", nil)
	})
	router.Run(":12121")
}

func ShowImage(c *gin.Context) {
	imageName := c.Query("imageName")
	c.File(imageName)
}

func SendSms(c *gin.Context) {
	mobile := c.Query("mobile")
	code := 0
	msg := ""
	t := time.Now().UnixNano() / 1000000
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

func DragElement(sel interface{}, loc_x float64) chromedp.QueryAction {
	return chromedp.QueryAfter(sel, func(ctx context.Context, id runtime.ExecutionContextID, node ...*cdp.Node) error {
		if len(node) == 0 {
			return fmt.Errorf("找不到相关 Node")
		}
		return MouseDragNode(node[0], loc_x).Do(ctx)
	})
}

func MouseDragNode(n *cdp.Node, loc_x float64) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		boxes, err := dom.GetContentQuads().WithNodeID(n.NodeID).Do(ctx)
		if err != nil {
			return err
		}

		box := boxes[0]
		c := len(box)
		if c%2 != 0 || c < 1 {
			return chromedp.ErrInvalidDimensions
		}

		var x, y float64
		for i := 0; i < c; i += 2 {
			x += box[i]
			y += box[i+1]
		}
		x /= float64(c / 2)
		y /= float64(c / 2)

		p := &input.DispatchMouseEventParams{
			Type:       input.MousePressed,
			X:          x,
			Y:          y,
			Button:     input.Left,
			ClickCount: 1,
		}

		if err := p.Do(ctx); err != nil {
			return err
		}
		mid1 := math.Round(loc_x * (rand.Float64()*10 + 10) / 100)
		mid2 := math.Round(loc_x * (rand.Float64()*11 + 65) / 100)
		mid3 := math.Round(loc_x * (rand.Float64()*4 + 84) / 100)

		current := 0.0
		v := 0.0
		t := 0.2
		i := 1
		log.Println(mid1, mid2, mid3)
		for current < loc_x {
			a := 0
			if current < mid1 {
				a = rand.Intn(10) + 30
			} else if current < mid2 {
				a = rand.Intn(30) + 50
			} else if current < mid3 {
				a = -70
			} else {
				a = rand.Intn(7) - 25
			}
			v0 := v
			v = v0 + float64(a)*t
			if v < 0 {
				v = 0
			}
			move_x := v0*t + float64(a)*t*t*1/2
			if move_x < 0 {
				move_x = 1
			} else {
				move_x = math.Round(move_x)
			}
			log.Println("move_x", move_x)

			move_y := float64(rand.Intn(6) - 2)

			for j := 0; j < 20; j++ {
				current += move_x / 20
				p.Type = input.MouseMoved
				p.X = x + current
				p.Y = y + move_y
				if err := p.Do(ctx); err != nil {
					log.Println(err)
					return err
				}

				time.Sleep(time.Millisecond * 10)
				// time.Sleep(time.Second * 10 / 1000)
				// log.Println("move", current, i)
				i++
			}
		}
		log.Println(current, loc_x)

		var back_tracks []float64
		out_range := loc_x - current
		if out_range < -8 {
			sub := out_range + 8
			back_tracks = append(back_tracks, -1, sub, -1, -1, -1, -1)
		} else if out_range < -2 {
			sub := out_range + 3
			back_tracks = append(back_tracks, -1, -1, sub)
		}
		time.Sleep(time.Millisecond * time.Duration(rand.Intn(440)+60))
		// time.Sleep(time.Second * time.Duration(rand.Intn(440)+60) / 1000)
		for _, v := range back_tracks {
			log.Println("move_x", v)
			move_y := float64(rand.Intn(4) - 2)
			for j := 0; j < 20; j++ {
				current += v / 20
				p.Type = input.MouseMoved
				p.X = x + current
				p.Y = y + move_y
				if err := p.Do(ctx); err != nil {
					log.Println(err)
					return err
				}
				time.Sleep(time.Millisecond * 10)
				// time.Sleep(time.Second * 10 / 1000)
				// log.Println("move", current, i)
				i++
			}
		}

		// current += rand.Float64() * -1.67
		// p.Type = input.MouseMoved
		// p.X = x + current
		// p.Y = y + float64(rand.Intn(2)-1)
		// if err := p.Do(ctx); err != nil {
		// 	log.Println(err)
		// 	return err
		// }

		// current += rand.Float64() * 1.67
		// p.Type = input.MouseMoved
		// p.X = x + current
		// p.Y = y + float64(rand.Intn(2)-1)
		// if err := p.Do(ctx); err != nil {
		// 	log.Println(err)
		// 	return err
		// }
		// time.Sleep(time.Second * time.Duration(rand.Intn(50)+150) / 1000)
		time.Sleep(time.Millisecond * time.Duration(rand.Intn(50)+150))
		p.Type = input.MouseReleased
		return p.Do(ctx)
	}
}

func handle(mobile string) {
	defer func() {
		timestamp = 0
		mu.Lock()
		used = false
		mu.Unlock()
	}()
	var msg HandleMsg
	// //本地测试用
	// options := []chromedp.ExecAllocatorOption{
	// 	chromedp.Flag("headless", false),
	// }
	// options = append(chromedp.DefaultExecAllocatorOptions[:], options...)
	// ctx, cancel := chromedp.NewExecAllocator(context.Background(), options...)
	// defer cancel()
	// ctx, cancel = chromedp.NewContext(ctx)
	// defer cancel()

	//线上用
	ctx, cancel := chromedp.NewContext(
		context.Background(),
		chromedp.WithLogf(log.Printf),
	)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	var btnText string
	isExist := false
	err := chromedp.Run(ctx, startJD(), sendKeys(".mobile", mobile),
		chromedp.Click("document.querySelector('.getMsg-btn')", chromedp.ByJSPath),
		chromedp.Sleep(time.Second),
	)
	if err != nil {
		msg.Code = 999
		msg.Msg = err.Error()
		log.Println(err.Error())
		chMsg <- msg
		return
	}
	count := 0
	for {

		chromedp.Run(ctx, chromedp.EvaluateAsDevTools("document.getElementById('captcha_modal')!=null", &isExist))
		log.Println("isExist", isExist)
		if isExist {
			count++
			if count > 15 {
				msg.Code = 3
				msg.Msg = "滑块验证失败"
				chMsg <- msg
				return
			}
			log.Println("第", count, "次验证")
			move(ctx)
		} else {
			break
		}
	}
	time.Sleep(time.Second)
	chromedp.Run(ctx, chromedp.Text(`.getMsg-btn`, &btnText, chromedp.NodeVisible))
	log.Println(btnText)
	if !strings.HasPrefix(btnText, "重新获取") {
		msg.Code = 3
		// var dialogDes string
		// chromedp.Run(ctx,
		// 	chromedp.Text(`.dialog-des`, &dialogDes, chromedp.NodeVisible),
		// )
		// if dialogDes != "" {
		// 	msg.Msg = dialogDes
		// } else {

		//todo 获取错误

		msg.Msg = "未知错误"
		var bufs []byte
		chromedp.Run(ctx, chromedp.CaptureScreenshot(&bufs))
		if err := ioutil.WriteFile("error.fullScreenshot.png", bufs, 0644); err != nil {
			log.Println(err)
		}
		// }
		chMsg <- msg
		return
	}
	var checked bool
	//获取不到值 换种方式
	// var ok bool
	// chromedp.Run(ctx, chromedp.AttributeValue("document.querySelector('.policy_tip-checkbox')", "checked", &checked, &ok, chromedp.ByJSPath))

	chromedp.Run(ctx, chromedp.EvaluateAsDevTools("document.querySelector('.policy_tip-checkbox').checked", &checked))

	log.Println("协议", checked)
	if !checked {
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
			err := chromedp.Run(ctx, sendKeys("#authcode", code), chromedp.Click("document.querySelector('.btn')", chromedp.ByJSPath), chromedp.Sleep(time.Second*2))
			if err != nil {
				msg.Code = 999
				msg.Msg = err.Error()
				log.Println(err.Error())
				// restartHeadlessShellDocker()
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

func move(ctx context.Context) {
	log.Println("滑块")
	var cpc_img, small_img string
	var ok bool
	image_width := 0.0
	chromedp.Run(ctx,
		chromedp.Sleep(time.Second),
		chromedp.AttributeValue("#cpc_img", "src", &cpc_img, &ok, chromedp.ByID),
		chromedp.AttributeValue("#small_img", "src", &small_img, &ok, chromedp.ByID),
		chromedp.EvaluateAsDevTools("document.getElementById('cpc_img').width", &image_width),
	)
	if cpc_img == "" || small_img == "" {
		log.Println("image is null")
		return
	}
	resp, err := grpcClient.GetDistance(ctx, &pb.Request{CpcImg: cpc_img, SmallImg: small_img})
	if err != nil {
		log.Println(err)
		return
	}
	// //保存图片
	// utils.SaveBase64ToFile(strings.TrimPrefix(cpc_img, "data:image/jpg;base64,"), "./r1.jpg")
	// utils.SaveBase64ToFile(strings.TrimPrefix(small_img, "data:image/png;base64,"), "./r2.png")
	// //灰度图片
	// gocv.IMWrite("r3.jpg", gocv.IMRead("r1.jpg", gocv.IMReadGrayScale))
	// gocv.IMWrite("r4.jpg", gocv.IMRead("r2.png", gocv.IMReadGrayScale))
	// var result = gocv.NewMat()
	// gocv.MatchTemplate(gocv.IMRead("r4.jpg", gocv.IMReadUnchanged), gocv.IMRead("r3.jpg", gocv.IMReadUnchanged), &result, gocv.TmCcoeffNormed, gocv.NewMat())
	// gocv.Normalize(result, &result, 0, 1, gocv.NormMinMax)

	// _, _, _, maxLoc := gocv.MinMaxLoc(result)

	chromedp.Run(ctx, DragElement(".sp_msg img", float64(resp.Distance)/275*image_width-1))
	time.Sleep(time.Second * 2)
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
		tasks = append(tasks, chromedp.SendKeys(sel, string(s), chromedp.NodeVisible), chromedp.Sleep(time.Millisecond*10))
		// tasks = append(tasks, chromedp.SendKeys(sel, string(s), chromedp.NodeVisible), chromedp.Sleep(time.Second*10/1000))
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
