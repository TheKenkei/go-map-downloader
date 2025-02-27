package main

import (
	"fmt"
	"math"
	"math/rand"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Icemap/coordinate"
	"github.com/cheggaaa/pb/v3"
	"github.com/panjf2000/ants/v2"
)

type Task struct {
	err      error
	x        int
	y        int
	z        int
	retryNum int
	config   MapConfig
}

func main() {
	mask := syscall.Umask(0)
	defer syscall.Umask(mask)

	config := parseMapConfig()
	fmt.Printf("config: %+v\n", config)

	fmt.Println("start calculate...")
	// calculate
	leftTop, err := coordinate.Convert(coordinate.WGS84, coordinate.WebMercator, coordinate.Coordinate{X: config.LeftLongitude, Y: config.TopLatitude})
	if err != nil {
		panic(fmt.Errorf("left top error: %+v", err))
	}

	rightBottom, err := coordinate.Convert(coordinate.WGS84, coordinate.WebMercator, coordinate.Coordinate{X: config.RightLongitude, Y: config.BottomLatitude})
	if err != nil {
		panic(fmt.Errorf("right bottom error: %+v", err))
	}

	fmt.Printf("leftTop: %+v, rightBottom: %+v\n", leftTop, rightBottom)

	totalTask := 0
	for z := config.MinLevel; z <= config.MaxLevel; z++ {
		perTileWidth := coordinate.WebMercatorWidth / math.Pow(2, float64(z))
		leftTileX := int(leftTop.X / perTileWidth)
		rightTileX := int(rightBottom.X / perTileWidth)
		topTileY := int(leftTop.Y / perTileWidth)
		bottomTileY := int(rightBottom.Y / perTileWidth)
		totalTask += (rightTileX - leftTileX + 1) * (bottomTileY - topTileY + 1)
	}

	// download
	fmt.Println("start download...")
	bar := pb.StartNew(totalTask)
	var wg sync.WaitGroup

	pool, _ := ants.NewPoolWithFunc(config.GoroutineNum, func(iTask interface{}) {
		defer func() {
			wg.Done()
			bar.Increment()
		}()

		task, _ := iTask.(Task)
		err := getPic(task)
		for i := 0; i < task.config.MaxRetryNum; i++ {
			if err != nil {
				fmt.Printf("retry num: %d, task: %+v\n", i, task)
				err = getPic(task)
			}
		}
	})
	defer pool.Release()

	currentTask := 0
	for z := config.MinLevel; z <= config.MaxLevel; z++ {
		perTileWidth := coordinate.WebMercatorWidth / math.Pow(2, float64(z))
		leftTileX := int(leftTop.X / perTileWidth)
		rightTileX := int(rightBottom.X / perTileWidth)
		topTileY := int(leftTop.Y / perTileWidth)
		bottomTileY := int(rightBottom.Y / perTileWidth)

		currentTask++
		if currentTask%config.QPS == 0 {
			time.Sleep(time.Second)
		}

		for x := leftTileX; x <= rightTileX; x++ {
			for y := topTileY; y <= bottomTileY; y++ {
				wg.Add(1)
				pool.Invoke(Task{
					x:        x,
					y:        y,
					z:        z,
					retryNum: 0,
					config:   config,
				})
			}
		}
	}

	wg.Wait()
	bar.Finish()

	// combine
	fmt.Println("start combine...")
	if config.Combine {
		return
	}
	for z := config.MinLevel; z <= config.MaxLevel; z++ {
		perTileWidth := coordinate.WebMercatorWidth / math.Pow(2, float64(z))
		leftTileX := int(leftTop.X / perTileWidth)
		rightTileX := int(rightBottom.X / perTileWidth)
		topTileY := int(leftTop.Y / perTileWidth)
		bottomTileY := int(rightBottom.Y / perTileWidth)

		err := combine(leftTileX, rightTileX, topTileY, bottomTileY, z, config)
		if err != nil {
			fmt.Printf("combine pic error: %+v\n", err)
		}
	}
}

func getPic(task Task) error {
	if task.retryNum >= task.config.MaxRetryNum {
		fmt.Printf("task retry reached retry max time, err: %+v, task info: %+v\n", task.err, task)
		return nil
	}

	filePath := getPath(task.config, task.x, task.y, task.z)
	url := WebMercatorTileToURLWithTiltStyle("YMapImage", task.x, task.y, task.z,
		TiltStyle{GoogleWithLabel: task.config.GoogleWithLabel})

	return Download(url, filePath)
}

func getPath(config MapConfig, x, y, z int) string {
	return strings.Join([]string{
		config.SavePath, config.MapType, strconv.Itoa(z), strconv.Itoa(x), strconv.Itoa(y),
	}, string(filepath.Separator)) + ".jpg"
}

func getCombinePicPath(config MapConfig, z int) string {
	return strings.Join([]string{
		config.SavePath, config.MapType, "level_" + strconv.Itoa(z),
	}, string(filepath.Separator)) + ".jpg"
}

// Map URL
const (
	// GoogleSatelliteURL google.com/maps satellite map URL
	GoogleSatelliteURL = "http://mt[0,1,2,3].google.com/vt/lyrs=y&x={x}&y={y}&z={z}&s=Gali"
	// GoogleImageURL google.com/maps image map URL
	GoogleImageURL = "http://mt[0,1,2,3].google.com/vt/lyrs=m&gl=CN&x={x}&y={y}&z={z}&s=Gali"
	// GoogleTerrainURL google.com/maps terrain map URL
	GoogleTerrainURL = "http://mt[0,1,2,3].google.com/vt/lyrs=p&gl=CN&x={x}&y={y}&z={z}&s=Gali"
	// AMapSatelliteURL amap.com satellite map URL
	AMapSatelliteURL = "http://webst0[1,2,3,4].is.autonavi.com/appmaptile?style=6&x={x}&y={y}&z={z}"
	// AMapCoverURL amap.com cover map URL
	AMapCoverURL = "http://webst0[1,2,3,4].is.autonavi.com/appmaptile?x={x}&y={y}&z={z}&lang=zhcn&size=1&scale=1&style=8"
	// AMapImageURL amap.com image map URL
	AMapImageURL = "http://webrd0[1,2,3,4].is.autonavi.com/appmaptile?lang=zh_cn&size=1&scale=1&style=8&x={x}&y={y}&z={z}"
	// https://sat01.maps.yandex.net/tiles?l=sat&v=3.1521.0&x=620&y=318&z=10&lang=ru_RU
	// https://sat03.maps.yandex.net/tiles?l=sat&v=3.1521.0&x=621&y=320&z=10&lang=ru_RU
	// YMapImageURL = "https://sat02.maps.yandex.net/tiles?l=sat&v=3.1521.0&x={x}&y={y}&z={z}&lang=ru_RU&scale=2&dark=true"
	YMapImageURL = "https://core-renderer-tiles.maps.yandex.net/tiles?l=map&x={x}&y={y}&z={z}&scale=2&lang=ru_RU&theme=dark"

	// GoogleWithoutLabelSuffix google map add this suffix to drop the label
	GoogleWithoutLabelSuffix = "&apistyle=s.t%3A0%7Cs.e%3Al%7Cp.v%3Aoff"
)

// Map Type
const (
	// GoogleSatellite google.com/maps satellite map
	GoogleSatellite = "GoogleSatellite"
	// GoogleImage google.com/maps image map
	GoogleImage = "GoogleImage"
	// GoogleTerrain google.com/maps terrain map
	GoogleTerrain = "GoogleTerrain"
	// AMapSatellite amap.com satellite map
	AMapSatellite = "AMapSatellite"
	// AMapCover amap.com cover map
	AMapCover = "AMapCover"
	// AMapImage amap.com image map
	AMapImage = "AMapImage"
	YMapImage = "YMapImage"
)

type TiltStyle struct {
	// Google Specify Config

	// Google map tiles without label
	GoogleWithLabel bool
}

// WebMercatorTileToURLWithTiltStyle
// Convert web mercator tile number to URL, get domain randomly.
// And set the tilt style parameters.
func WebMercatorTileToURLWithTiltStyle(mapType string, x, y, z int,
	tiltStyle TiltStyle,
) string {
	urlModel := ""
	switch mapType {
	case GoogleSatellite:
		urlModel = GoogleSatelliteURL
	case GoogleImage:
		urlModel = GoogleImageURL
	case GoogleTerrain:
		urlModel = GoogleTerrainURL
	case AMapSatellite:
		urlModel = AMapSatelliteURL
	case AMapCover:
		urlModel = AMapCoverURL
	case AMapImage:
		urlModel = AMapImageURL
	case YMapImage:
		urlModel = YMapImageURL
	}

	urlModel = strings.Replace(urlModel, "{x}", strconv.Itoa(x), 1)
	urlModel = strings.Replace(urlModel, "{y}", strconv.Itoa(y), 1)
	urlModel = strings.Replace(urlModel, "{z}", strconv.Itoa(z), 1)

	reg := regexp.MustCompile(`\[.*\]`)
	found := string(reg.Find([]byte(urlModel)))
	found = strings.TrimSuffix(strings.TrimPrefix(found, "["), "]")
	subArray := strings.Split(found, ",")
	urlModel = string(reg.ReplaceAll([]byte(urlModel), []byte(subArray[rand.Intn(len(subArray))])))

	// Google type
	if mapType == GoogleSatellite || mapType == GoogleImage || mapType == GoogleTerrain {
		if !tiltStyle.GoogleWithLabel {
			urlModel += GoogleWithoutLabelSuffix
		}
	}

	return urlModel
}

// WebMercatorTileToURL convert web mercator tile number to URL, get domain randomly
func WebMercatorTileToURL(mapType string, x, y, z int) string {
	return WebMercatorTileToURLWithTiltStyle(mapType, x, y, z, TiltStyle{GoogleWithLabel: true})
}
