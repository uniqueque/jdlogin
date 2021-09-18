```
docker pull chromedp/headless-shell:latest

docker run -d -p 9222:9222 --rm --name headless-shell chromedp/headless-shell

go run main.go
```
## 教程不写了