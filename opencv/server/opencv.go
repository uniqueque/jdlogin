package server

import (
	"context"
	"fmt"
	pb "jdlogin/proto"
	"jdlogin/utils"
	"strings"

	"gocv.io/x/gocv"
)

type OpenCVServer struct{}

func (s *OpenCVServer) GetDistance(ctx context.Context, req *pb.Request) (*pb.Response, error) {
	resp := pb.Response{}
	var err error
	if req.SmallImg == "" || req.CpcImg == "" {
		resp.Distance = -1
		err = fmt.Errorf("invalid request")
	} else {
		//保存图片
		utils.SaveBase64ToFile(strings.TrimPrefix(req.CpcImg, "data:image/jpg;base64,"), "./r1.jpg")
		utils.SaveBase64ToFile(strings.TrimPrefix(req.SmallImg, "data:image/png;base64,"), "./r2.png")
		//灰度图片
		gocv.IMWrite("r3.jpg", gocv.IMRead("r1.jpg", gocv.IMReadGrayScale))
		gocv.IMWrite("r4.jpg", gocv.IMRead("r2.png", gocv.IMReadGrayScale))
		var result = gocv.NewMat()
		gocv.MatchTemplate(gocv.IMRead("r4.jpg", gocv.IMReadUnchanged), gocv.IMRead("r3.jpg", gocv.IMReadUnchanged), &result, gocv.TmCcoeffNormed, gocv.NewMat())
		gocv.Normalize(result, &result, 0, 1, gocv.NormMinMax)

		_, _, _, maxLoc := gocv.MinMaxLoc(result)
		resp.Distance = int64(maxLoc.X)
	}
	return &resp, err
}
