// What it does:
//
// This example detects motion using a delta threshold from the first frame,
// and then finds contours to determine where the object is located.
//
// Very loosely based on Adrian Rosebrock code located at:
// http://www.pyimagesearch.com/2015/06/01/home-surveillance-and-motion-detection-with-the-raspberry-pi-python-and-opencv/
//
// How to run:
//
// 		go run ./cmd/motion-detect/main.go 0
//

package main

import (
	"fmt"
	"image"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"gocv.io/x/gocv"
)

type AudioFile struct {
	streamer beep.StreamSeekCloser
	format   beep.Format
	filename string
}

const MinimumArea = 3000

var directory string

func main() {

	directory = filepath.FromSlash("audios/")

	files, err := ioutil.ReadDir(directory)
	if err != nil {
		log.Fatal(err)
	}
	filenames := []string{}
	for _, file := range files {
		if !file.IsDir() {
			filename := strings.ToLower(file.Name())
			if string(filename[len(filename)-4:]) == ".mp3" {
				filenames = append(filenames, file.Name())
			}
		}
	}

	qtyAudios := len(filenames)

	if qtyAudios == 0 {
		log.Fatal("We can't find audio files")
	}
	if qtyAudios == 1 {
		log.Fatal("We need more than 1 audio file")
	}

	log.Printf("We found %d audio files", len(filenames))

	audios, err := prepareAudios(filenames)
	if err != nil {
		log.Fatal("We can't prepare audios")
	}

	defer func() {
		for _, a := range audios {
			a.streamer.Close()
		}
	}()

	rand.Seed(time.Now().UnixMicro())

	deviceID := 0

	webcam, err := gocv.OpenVideoCapture(deviceID)
	if err != nil {
		fmt.Printf("Error opening video capture device: %v\n", deviceID)
		return
	}
	defer webcam.Close()

	window := gocv.NewWindow("Motion Window")
	defer window.Close()

	img := gocv.NewMat()
	defer img.Close()

	imgDelta := gocv.NewMat()
	defer imgDelta.Close()

	imgThresh := gocv.NewMat()
	defer imgThresh.Close()

	mog2 := gocv.NewBackgroundSubtractorMOG2()
	defer mog2.Close()

	play := make(chan bool, 1)

	for {
		if ok := webcam.Read(&img); !ok {
			log.Printf("Device closed: %v\n", deviceID)
			return
		}
		if img.Empty() {
			continue
		}
		// first phase of cleaning up image, obtain foreground only
		mog2.Apply(img, &imgDelta)

		// remaining cleanup of the image to use for finding contours.
		// first use threshold
		gocv.Threshold(imgDelta, &imgThresh, 25, 255, gocv.ThresholdBinary)

		// then dilate
		kernel := gocv.GetStructuringElement(gocv.MorphRect, image.Pt(3, 3))
		gocv.Dilate(imgThresh, &imgThresh, kernel)
		kernel.Close()

		// now find contours
		contours := gocv.FindContours(imgThresh, gocv.RetrievalExternal, gocv.ChainApproxSimple)

		for i := 0; i < contours.Size(); i++ {
			area := gocv.ContourArea(contours.At(i))
			if area < MinimumArea {
				continue
			}
			if cap(play) != len(play) {
				go playAudio(play, audios)
				play <- true
			}
		}

		contours.Close()

		window.IMShow(img)
		if window.WaitKey(1) == 27 {
			break
		}
	}
}

func playAudio(c chan bool, audios []AudioFile) {
	i := rand.Intn(len(audios))
	err := speaker.Init(audios[i].format.SampleRate, audios[i].format.SampleRate.N(time.Second/10))
	if err != nil {
		return
	}
	speaker.Play(beep.Seq(audios[i].streamer, beep.Callback(func() {
		audios[i].streamer.Seek(0)
		log.Println("Audio played:", audios[i].filename)
		time.Sleep(time.Second * 5)
		<-c
	})))
}

func prepareAudios(files []string) ([]AudioFile, error) {

	result := []AudioFile{}

	for _, f := range files {
		filename := directory + f
		of, err := os.Open(filename)
		if err != nil {
			return nil, err
		}

		streamer, format, err := mp3.Decode(of)
		if err != nil {
			return nil, err
		}

		result = append(result, AudioFile{streamer, format, filename})
	}
	return result, nil
}
