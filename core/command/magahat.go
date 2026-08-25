package command

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"math"
	"math/rand"
	"time"

	pigo "github.com/esimov/pigo/core"
	"github.com/fogleman/gg"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"qshqn/core/locale"
	"qshqn/core/qsh"
)

//go:embed assets/facefinder
var facefinderCascade []byte
var pigoCascade *pigo.Pigo
var magahatQueue = make(chan struct{}, 2)

func init() {
	var queueTimeout = 10 * time.Second

	ID := "magahat"
	otherMsgs := struct {
		NotAvailable,
		QueueTimeout,
		NoImageFound,
		NoFaceFound,
		ProcessingError locale.MsgID
	}{}

	var err error
	pigoCascade, err = pigo.NewPigo().Unpack(facefinderCascade)
	if err != nil {
		qsh.Errorf("magahat: failed to unpack pigo cascade: %w", err)
	}

	extractPhotoLocation := func(ctx *Context) (*tg.InputPhotoFileLocation, error) {
		msg := ctx.Message
		if msg == nil {
			return nil, fmt.Errorf("no message")
		}

		if msg.Media == nil && msg.ReplyTo != nil {
			if header, ok := msg.ReplyTo.(*tg.MessageReplyHeader); ok && header.ReplyToMsgID != 0 {
				var resp tg.MessagesMessagesClass
				var err error
				if channel, ok := ctx.InputPeer.(*tg.InputPeerChannel); ok {
					resp, err = ctx.Api.ChannelsGetMessages(ctx.Ctx, &tg.ChannelsGetMessagesRequest{
						Channel: &tg.InputChannel{
							ChannelID:  channel.ChannelID,
							AccessHash: channel.AccessHash,
						},
						ID: []tg.InputMessageClass{&tg.InputMessageID{ID: header.ReplyToMsgID}},
					})
				} else {
					resp, err = ctx.Api.MessagesGetMessages(ctx.Ctx, []tg.InputMessageClass{&tg.InputMessageID{ID: header.ReplyToMsgID}})
				}

				if err == nil && resp != nil {
					var msgs []tg.MessageClass
					switch r := resp.(type) {
					case *tg.MessagesMessages:
						msgs = r.Messages
					case *tg.MessagesMessagesSlice:
						msgs = r.Messages
					case *tg.MessagesChannelMessages:
						msgs = r.Messages
					}
					if len(msgs) > 0 {
						if m, ok := msgs[0].(*tg.Message); ok {
							msg = m
						}
					}
				}
			}
		}

		if msg == nil || msg.Media == nil {
			return nil, fmt.Errorf("no media found")
		}

		m, ok := msg.Media.(*tg.MessageMediaPhoto)
		if !ok {
			return nil, fmt.Errorf("media is not photo")
		}

		photo, ok := m.Photo.(*tg.Photo)
		if !ok {
			return nil, fmt.Errorf("photo is empty")
		}

		var largestType string
		for _, sz := range photo.Sizes {
			if s, ok := sz.(*tg.PhotoSize); ok {
				largestType = s.Type
			}
		}

		return &tg.InputPhotoFileLocation{
			ID:            photo.ID,
			AccessHash:    photo.AccessHash,
			FileReference: photo.FileReference,
			ThumbSize:     largestType,
		}, nil
	}

	drawPythonMagaHat := func(dc *gg.Context, det pigo.Detection, idx int) {
		w := det.Scale
		x := det.Col - det.Scale/2
		y := det.Row - det.Scale/2

		drawingWidth := max(1, w/18)
		lineEndX := x + w

		circleRadius := w / 3
		halfCircleRadius := circleRadius / 2
		circleCenterX := lineEndX - circleRadius - halfCircleRadius

		lineDrawingSteps := max(1, drawingWidth)
		circleDrawingSteps := max(1, drawingWidth)
		numFillLines := 20
		yOffsetMax := max(1, w/50)

		// qsh.Debugf("magahat [%d]: col=%d row=%d scale=%d (x=%d y=%d w=%d) drawWidth=%d radius=%d",
		// 	idx, det.Col, det.Row, det.Scale, x, y, w, drawingWidth, circleRadius)

		dc.SetRGB255(255, 0, 0)
		dc.SetLineWidth(float64(drawingWidth))

		for i := x - halfCircleRadius; i < lineEndX-halfCircleRadius; i += lineDrawingSteps {
			yOffset := rand.Intn(2*yOffsetMax+1) - yOffsetMax
			dc.DrawLine(float64(i), float64(y+yOffset), float64(i+lineDrawingSteps), float64(y+yOffset))
			dc.Stroke()
		}

		for angle := 0; angle < 180; angle += circleDrawingSteps {
			rad := float64(angle) * math.Pi / 180.0
			nextRad := float64(angle+circleDrawingSteps) * math.Pi / 180.0

			startX := float64(circleCenterX) + float64(circleRadius)*math.Cos(rad)
			startY := float64(y) - float64(circleRadius)*math.Sin(rad)
			endX := float64(circleCenterX) + float64(circleRadius)*math.Cos(nextRad)
			endY := float64(y) - float64(circleRadius)*math.Sin(nextRad)

			dc.DrawLine(startX, startY, endX, endY)
			dc.Stroke()
		}

		for range numFillLines {
			startX := float64(circleCenterX - circleRadius + rand.Intn(2*circleRadius+1))
			startY := float64(y)
			angleOffset := (rand.Float64()*0.6 - 0.3)
			rad := (90.0 + angleOffset*180.0) * math.Pi / 180.0

			endX := float64(circleCenterX) + float64(circleRadius)*math.Cos(rad)
			endY := float64(y) - float64(circleRadius)*math.Sin(rad)

			dc.DrawLine(startX, startY, endX, endY)
			dc.Stroke()
		}
	}

	exec := func(ctx *Context) (passthrough bool, err error) {
		if pigoCascade == nil {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.NotAvailable)
			return false, err
		}

		select {
		case magahatQueue <- struct{}{}:
			defer func() { <-magahatQueue }()
		case <-ctx.Ctx.Done():
			return false, ctx.Ctx.Err()
		case <-time.After(queueTimeout):
			_, err := ctx.ReplyReportErrLocaleMsgf(otherMsgs.QueueTimeout, locale.KV("seconds", int(queueTimeout.Seconds())))
			return false, err
		}

		stopTyping := ctx.StartTyping()
		defer stopTyping()

		// t0 := time.Now()

		photoLoc, err := extractPhotoLocation(ctx)
		if err != nil || photoLoc == nil {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.NoImageFound)
			return false, err
		}

		dl := downloader.NewDownloader()
		var imgBuf bytes.Buffer
		_, err = dl.Download(ctx.Api, photoLoc).Stream(ctx.Ctx, &imgBuf)
		if err != nil {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.ProcessingError)
			return false, fmt.Errorf("image download error: %w", err)
		}

		srcImg, _, err := image.Decode(&imgBuf)
		if err != nil {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.ProcessingError)
			return false, fmt.Errorf("image decode error: %w", err)
		}

		pixels := pigo.RgbToGrayscale(pigo.ImgToNRGBA(srcImg))
		bounds := srcImg.Bounds()
		cols, rows := bounds.Dx(), bounds.Dy()

		cParams := pigo.CascadeParams{
			MinSize:     30,
			MaxSize:     1000,
			ShiftFactor: 0.1,
			ScaleFactor: 1.1,
			ImageParams: pigo.ImageParams{
				Pixels: pixels,
				Rows:   rows,
				Cols:   cols,
				Dim:    cols,
			},
		}

		dets := pigoCascade.RunCascade(cParams, 0)
		clustered := pigoCascade.ClusterDetections(dets, 0.2)
		var detections []pigo.Detection
		for _, d := range clustered {
			if d.Q >= 5.0 {
				detections = append(detections, d)
			}
		}

		// qsh.Debugf("magahat: img=%dx%d raw_dets=%d clustered_dets=%d (took %.2fms)",
		// 	cols, rows, len(dets), len(detections), time.Since(t0).Seconds()*1000)

		if len(detections) == 0 {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.NoFaceFound)
			return false, err
		}

		dc := gg.NewContextForImage(srcImg)
		for i, det := range detections {
			drawPythonMagaHat(dc, det, i+1)
		}

		var outBuf bytes.Buffer
		if err := png.Encode(&outBuf, dc.Image()); err != nil {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.ProcessingError)
			return false, err
		}

		up := uploader.NewUploader(ctx.Api)
		uploadedFile, err := up.FromBytes(ctx.Ctx, "magahat.png", outBuf.Bytes())
		if err != nil {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.ProcessingError)
			return false, fmt.Errorf("image upload error: %w", err)
		}

		_, err = ctx.ReplyMediaRaw(message.UploadedPhoto(uploadedFile))
		// qsh.Debugf("magahat: total request time %.2fs", time.Since(t0).Seconds())
		return false, err
	}

	register(ID, &otherMsgs, exec)
}
