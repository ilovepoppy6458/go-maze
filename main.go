//package main 迷宫游戏
package main

import (
	crand "crypto/rand"
	"encoding/csv"
	"fmt"
	"image"
	"io"
	"math"
	"math/big"
	"math/rand"
	"os"
	"strconv"
	"time"

	"image/color"
	_ "image/png"

	"github.com/faiface/pixel"
	"github.com/faiface/pixel/imdraw"
	"github.com/faiface/pixel/pixelgl"
	"github.com/pkg/errors"
	"golang.org/x/image/colornames"
)

var (
	//defaultBound 默认窗口大小
	defaultBound pixel.Rect = pixel.R(0, 0, 1024, 1024)
	//defaultN 迷宫格子每行和每列多少个
	defaultN int = 10
	//latticeNum 迷宫格子数量
	latticeNum int = defaultN * defaultN
	//defaultDis 每个格子的宽度
	defaultDis float64 = float64(1024) / float64(defaultN)
)

//玩家状态
const (
	idle int = iota
	running
	jumping
	uping
)

//玩家移动方向
const (
	stand int = iota
	left
	up
	right
	down
)

//TargAndPict pixel.Target和pixel.Picture
type TargAndPict interface {
	pixel.Target
	pixel.Picture
}

//Goal 循环旋转的4个球表示出口
type Goal struct {
	pos     pixel.Vec
	radius  float64
	chpos   []pixel.Vec
	chcolor []color.RGBA
}

//Init 初始化4个孩子球
func (gol *Goal) Init() {
	gol.chpos = append(gol.chpos, pixel.V(gol.pos.X, gol.pos.Y+gol.radius/2))
	gol.chpos = append(gol.chpos, pixel.V(gol.pos.X+gol.radius/2, gol.pos.Y))
	gol.chpos = append(gol.chpos, pixel.V(gol.pos.X, gol.pos.Y-gol.radius/2))
	gol.chpos = append(gol.chpos, pixel.V(gol.pos.X-gol.radius/2, gol.pos.Y))

	gol.chcolor = append(gol.chcolor, colornames.Blue)
	gol.chcolor = append(gol.chcolor, colornames.Black)
	gol.chcolor = append(gol.chcolor, colornames.White)
	gol.chcolor = append(gol.chcolor, colornames.Red)
}

//Draw 循环交换颜色绘制圆
func (gol *Goal) Draw(imd *imdraw.IMDraw) {
	gol.chcolor = append(gol.chcolor[1:], gol.chcolor[0])
	for i := 0; i < 4; i++ {
		imd.Color = gol.chcolor[i]
		imd.Push(gol.chpos[i])
		imd.Circle(gol.radius/2, 0)
	}
}

//Ground 背景图
type Ground struct {
	sprite *pixel.Sprite
	pic    pixel.Picture
	bound  pixel.Rect
	win    TargAndPict
}

//Init 初始化sprite
func (gr *Ground) Init() {
	gr.sprite = pixel.NewSprite(gr.pic, gr.bound)
}

//Player 玩家
type Player struct {
	sprite  *pixel.Sprite
	canvas  *pixelgl.Canvas
	wls     *Walls
	vec     pixel.Vec
	pic     pixel.Picture
	anims   map[string][]pixel.Rect
	frame   pixel.Rect
	counter float64
	pos     pixel.Vec
	state   int
	dire    int
	speed   int
	change  int
}

//update 更新参数
func (pl *Player) update(dt float64) {
	last := pl.pos
	i := int(math.Floor(float64(last.X)/defaultDis)) + int(math.Floor(float64(last.Y)/defaultDis))*defaultN
	//fmt.Printf("i:%v\n", i)
	bound := [4]float64{float64(i%defaultN) * defaultDis, float64(i/defaultN+1) * defaultDis, float64(i%defaultN+1) * defaultDis, float64(i/defaultN) * defaultDis}
	//fmt.Printf("bound:%v\n", bound)
	//fmt.Printf("dire:%v\n", pl.dire)
	vel := pixel.ZV
	if pl.dire == left {
		vel.X = float64(-pl.speed)
		if i%defaultN == 0 || pl.wls.wall[2*i-1] == true {
			if pl.pos.X+vel.X <= bound[0] {
				vel.X = bound[0] - pl.pos.X
			}
		}
	}
	if pl.dire == right {
		vel.X = float64(pl.speed)
		if (i+1)%defaultN == 0 || pl.wls.wall[2*i+1] == true {
			if pl.pos.X+pl.frame.W()+pl.wls.pic.Bounds().H()/2+vel.X >= bound[2] {
				vel.X = bound[2] - pl.pos.X - pl.frame.W() - pl.wls.pic.Bounds().H()/2
			}
		}
	}
	if pl.dire == up {
		vel.Y = float64(pl.speed)
		if i/defaultN+1 == defaultN || pl.wls.wall[2*i] == true {
			if pl.pos.Y+pl.frame.H()+pl.wls.pic.Bounds().H()/2+vel.Y >= bound[1] {
				vel.Y = bound[1] - pl.pos.Y - pl.frame.H() - pl.wls.pic.Bounds().H()/2
			}
		}
	}
	if pl.dire == down {
		vel.Y = float64(-pl.speed)
		if i/defaultN == 0 || pl.wls.wall[(i-defaultN)*2] == true {
			if pl.pos.Y+vel.Y <= bound[3] {
				vel.Y = bound[3] - pl.pos.Y
			}
		}
	}
	aimPos := pixel.Lerp(last, last.Add(vel), math.Pow(1.0/128, dt))
	pl.pos = aimPos
	//fmt.Printf("pos:%v\n", pl.pos)
	if pl.change != pl.state {
		pl.state = pl.change
		pl.counter = 0.0
	} else {
		pl.counter += dt
	}
	switch pl.state {
	case idle:
		pl.frame = pl.anims["Front"][0]
	case uping:
		pl.frame = pl.anims["FrontBlink"][0]
	case running:
		i := int(math.Floor(pl.counter / 0.1))
		pl.frame = pl.anims["Run"][i%len(pl.anims["Run"])]
	case jumping:
		i := int(math.Floor(pl.counter / 0.1))
		pl.frame = pl.anims["Jump"][i%len(pl.anims["Jump"])]
	}
	//fmt.Printf("state:%v,frame:%v\n", pl.state, pl.frame)
}

//Draw 画入将玩家
func (pl *Player) Draw(t pixel.Target) {
	if pl.sprite == nil {
		pl.sprite = pixel.NewSprite(nil, pixel.Rect{})
	}
	pl.sprite.Set(pl.pic, pl.frame)
	dr := 1.0
	if pl.dire == right {
		dr = -1.0
	}
	pl.sprite.Draw(t, pixel.IM.ScaledXY(pixel.ZV, pixel.V(dr, 1)).Moved(pl.vec))
}

//Walls 迷宫的墙
type Walls struct {
	sprite *pixel.Sprite
	pic    pixel.Picture
	bound  pixel.Rect
	wall   []bool
	wn     int
	win    TargAndPict
}

//Init 初始化墙
func (wls *Walls) Init() {
	wls.sprite = pixel.NewSprite(wls.pic, wls.bound)
	wls.wall = make([]bool, latticeNum*2)
	wls.wn = 0
}

//SetRandWalls 随机化一组迷宫墙wall:[]byte，值为1表示有墙，0则表示没墙。将窗口
//分割成N*N个格子，顺序为从左向右，从下到上，每个格子需要确定除了围墙之外的位置是
//否有墙，wall数组的内容按顺序指示每个格子的上方和右方是否有墙(占数组连续2个，第
//一个指示上方，第二个指示右方)，若其代表的是围墙，则其值固定为1
//       +-----------+
//    ^  | 7   8 | 9 |  示例:N=3
//    |  |---+   +   |  3*3个格子
//    |  | 4   5 | 6 |  wall数据为
//    |  |   +   +   |  [0,1,0,0,0,1,1,0,0,1,0,1,1,0,1,1,1,1]
//       | 1 | 2   3 |
//       +---+---+---+
//          ------>
func (wls *Walls) SetRandWalls() {
	//随机wall数组内容
	for i := 0; i < len(wls.wall); i++ {
		if isEnclosure(i) {
			wls.wall[i] = true
			continue
		}
		rd, _ := crand.Int(crand.Reader, big.NewInt(1024))
		if rd.Int64()/512 == 1 {
			wls.wall[i] = true
			wls.wn++
		} else {
			wls.wall[i] = false
		}
	}
}

//MakeMazeEvenly 修改wall让入口和出口连通，并使墙分布均匀
//默认入口为0，出口为N*N-1
func (wls *Walls) MakeMazeEvenly() {
	//sets[i][j]=true表示i和j连通
	sets := make([]map[int]bool, latticeNum)
	for i := 0; i < latticeNum; i++ {
		sets[i] = make(map[int]bool)
		sets[i][i] = true
	}
	//初始化sets
	for i := 0; i < len(wls.wall); i++ {
		if isEnclosure(i) {
			continue
		}
		if wls.wall[i] == false {
			lat := lattice(i)
			if i%2 == 0 {
				sets[lat][lat+defaultN] = true
				sets[lat+defaultN][lat] = true
			} else {
				sets[lat][lat+1] = true
				sets[lat+1][lat] = true
			}
		}
	}
	updateSets(&sets)
	fmt.Printf("len(sets[latticeNum-1]):%v\n", len(sets[latticeNum-1]))
	//若入口到出口未连通，则随机找到墙拆掉并更新sets
	l := len(sets[latticeNum-1])
	for l < latticeNum {
		//随机生成一个序列，找到第序列中第一个不与出口相通的格子
		t1 := time.Now()
		rand.Seed(time.Now().UnixNano())
		rl, ps, i, down := rand.Perm(latticeNum), -1, 0, false
		for ; i < len(rl); i++ {
			//fmt.Printf("lattice:%v\n", rl[i])
			if _, ok := sets[latticeNum-1][rl[i]]; ok {
				continue
			}
			//nb,bk分别表示格子按上右下左方向相邻的格子和格子的墙
			nb := []int{rl[i] + defaultN, rl[i] + 1, rl[i] - defaultN, rl[i] - 1}
			bk := []int{rl[i] * 2, rl[i]*2 + 1, (rl[i] - defaultN) * 2, rl[i]*2 - 1}
			min, index := make([]int, 0), make([]int, 0)
			//fmt.Printf("lattice not with latticeNum-1:%v\n", rl[i])
			//获取4个方向上的格子的连通格子数量，min按从小到大的顺序保存其值，index保存其下标
			for j := 0; j < 4; j++ {
				//格子合法，并且向右的不能是靠迷宫左墙，向左的不能靠右墙
				if validLattice(nb[j]) && validWall(bk[j]) &&
					((j == 1 && nb[j]%defaultN != 0) || (j == 3 && (nb[j]+1)%defaultN != 0)) {
					if wls.wall[bk[j]] == false {
						continue
					}
					k := 0
					for ; k < len(min); k++ {
						if min[k] >= len(sets[nb[j]]) {
							break
						}
					}
					if k >= len(min) {
						min = append(min, len(sets[nb[j]]))
						index = append(index, j)
					} else {
						t := min[k:]
						min = append(min[0:k], len(sets[nb[j]]))
						min = append(min, t...)
						t = index[k:]
						index = append(index[0:k], j)
						index = append(index, t...)
					}
				}
			}
			//拆掉连通邻居格子数量最小的格子的墙
			//fmt.Printf("min:%v\n", min)
			//fmt.Printf("index:%v\n", index)
			for k := 0; k < len(index); k++ {
				if _, ok := sets[rl[i]][nb[index[k]]]; !ok {
					wls.wall[bk[index[k]]] = false
					wls.wn--
					down = true
					ps = nb[index[k]]
					fmt.Printf("@%d pass %d to %d down %d\n", nb[index[k]], rl[i], nb[index[k]], bk[index[k]])
					break
				}
			}
			if down {
				break
			}
		}
		if i >= len(rl) {
			fmt.Println("i>=len(rl):")
			fmt.Printf("%v\n", rl)
			//panic("can't find a lattice")
			break
		}
		updateSetsByLat(&sets, rl[i], ps)
		//fmt.Printf("len(sets[latticeNum-1]):%v\n", len(sets[latticeNum-1]))
		l = len(sets[latticeNum-1])
		fmt.Printf("break:%v\n", time.Since(t1))
	}
	lns := constructLines(&wls.wall, nil)
	//fmt.Printf("%v\n", float64(wls.wn)/float64((latticeNum-defaultN)*2))
	canAdd := true
	//for defaultN == 10 && float64(wls.wn)/float64((latticeNum-defaultN)*2) < 0.45 {
	for canAdd && float64(wls.wn)/float64((latticeNum-defaultN)*2) < 0.45 {
		canAdd = false
		t1 := time.Now()
		rand.Seed(time.Now().UnixNano())
		rl := rand.Perm(latticeNum * 2)
		for i := 0; i < len(rl); i++ {
			//fmt.Printf("%v\n", float64(wls.wn)/float64((latticeNum-defaultN)*2))
			//printMaze(&(wls.wall))
			//fmt.Println("")
			if addWall(wls, lns, i) {
				fmt.Printf("%v\n", float64(wls.wn)/float64((latticeNum-defaultN)*2))
				//printMaze(&wls.wall)
				canAdd = true
				break
			}
		}
		fmt.Printf("addWall:%v\n", time.Since(t1))
	}
}

//updateSets 更新迷宫格子连通集合sets，时间复杂度O(n^3)，n=N*N
//依次更新格子0->n-1，更新与其连通的所有格子的所有连通格子的连通关系
func updateSets(sets *[]map[int]bool) {
	t1 := time.Now()
	for i := 0; i < len(*sets); i++ {
		for ki := range (*sets)[i] {
			for kj := range (*sets)[ki] {
				(*sets)[i][kj] = true
				(*sets)[kj][i] = true
			}
		}
	}
	fmt.Printf("updateSets:%v\n", time.Since(t1))
}

func updateSetsByLat(sets *[]map[int]bool, li int, lj int) {
	t1 := time.Now()
	for ki := range (*sets)[li] {
		for kj := range (*sets)[lj] {
			(*sets)[ki][kj] = true
			(*sets)[kj][ki] = true
		}
	}
	fmt.Printf("updateSetsByLat:%v\n", time.Since(t1))
}

//validLattice 格子没越界
func validLattice(lt int) bool {
	if lt >= 0 && lt < latticeNum {
		return true
	}
	return false
}

//validWall 墙没越界
func validWall(wl int) bool {
	if wl >= 0 && wl < latticeNum*2 {
		return true
	}
	return false
}

//lattice 返回墙所在的格子
func lattice(wl int) int {
	return wl / 2
}

//isEnclosure 返回某面墙是否是外围墙
func isEnclosure(wl int) bool {
	if (wl%2 == 1 && (lattice(wl)+1)%defaultN == 0) ||
		(wl%2 == 0 && lattice(wl) >= defaultN*(defaultN-1)) {
		return true
	}
	return false
}

//isLineToEnclosure 返回是否某面墙所在的格子是最外面一圈格子并且连接着围墙
func isLineToEnclosure(wl int) bool {
	if (wl%2 == 1 && (lattice(wl) < defaultN || lattice(wl) >= defaultN*(defaultN-1))) ||
		(wl%2 == 0 && (lattice(wl)%defaultN == 0 || (lattice(wl)+1)%defaultN == 0)) {
		return true
	}
	return false
}

//addWall 添加的墙不能与外墙构成环路，若添加墙i，返回true
//可以再增加墙之间的环路判断
func addWall(wls *Walls, lns *[]bool, i int) bool {
	if isEnclosure(i) || wls.wall[i] == true {
		return false
	}
	//与墙i相连的6个墙
	var nb []int
	if i%2 == 1 {
		nb = []int{i - 1, i + 2*defaultN, i + 1, i + 1 - 2*defaultN, i - 2*defaultN, i - 1 - 2*defaultN}
	} else {
		nb = []int{i - 2, i + 2*defaultN - 1, i + 2*defaultN + 1, i + 2, i + 1, i - 1}
	}
	count := 0
	//查看6个墙中有几个有到围墙的路径
	for j := 0; j < 6; j++ {
		if !validWall(nb[j]) || isEnclosure(nb[j]) {
			continue
		}
		if (*lns)[nb[j]] == true {
			count++
		}
	}
	//与围墙连通的墙小于1个则可以加墙
	if count == 0 || (count == 1 && !isLineToEnclosure(i)) {
		wls.wall[i] = true
		wls.wn++
		fmt.Printf("add wall:%v\n", i)
		//若邻墙有1个与围墙相连，则加墙的i也与围墙相连
		//if count == 0 {
		lns = constructLines(&wls.wall, lns)
		//}
		return true
	}
	return false
}

//constructLines 构造迷宫墙是否有连接到围墙的路径的bool数组
func constructLines(wls *[]bool, rs *[]bool) *[]bool {
	if rs == nil {
		ts := make([]bool, latticeNum*2)
		for i := 0; i < len(ts); i++ {
			ts[i] = false
		}
		rs = &ts
	}
	updated := true
	for updated {
		updated = false
		for i := 0; i < len(*wls); i++ {
			if isEnclosure(i) {
				continue
			}
			//若i与围墙相连则更新rs[i]
			if isLineToEnclosure(i) {
				if (*wls)[i] == true && (*rs)[i] != true {
					(*rs)[i] = true
					updated = true
				}
			} else { //否则若i的邻墙有连通围墙的路径则更新rs[i]
				//nb为与i相连的6个墙
				var nb []int
				if i%2 == 1 {
					nb = []int{i - 1, i + 2*defaultN, i + 1, i + 1 - 2*defaultN, i - 2*defaultN, i - 1 - 2*defaultN}
				} else {
					nb = []int{i - 2, i + 2*defaultN - 1, i + 2*defaultN + 1, i + 2, i + 1, i - 1}
				}
				for j := 0; j < 6; j++ {
					if !validWall(nb[j]) || isEnclosure(nb[j]) {
						continue
					}
					if (*wls)[i] == true && (*rs)[i] != true && (*rs)[nb[j]] == true {
						(*rs)[i] = true
						updated = true
						break
					}
				}
			}
		}
	}
	return rs
}

func printMaze(wls *[]bool) {
	for i := latticeNum - defaultN; i >= 0; i -= defaultN {
		fmt.Printf("\n+")
		for j := 0; j < defaultN; j++ {
			if (*wls)[(i+j)*2] == true {
				fmt.Printf("---+")
			} else {
				fmt.Printf("   +")
			}
		}
		fmt.Printf("\n|")
		for j := 0; j < defaultN; j++ {
			if (*wls)[(i+j)*2+1] == true {
				fmt.Printf("   |")
			} else {
				fmt.Printf("    ")
			}
		}
	}
	fmt.Printf("\n+")
	for i := 0; i < defaultN; i++ {
		fmt.Printf("---+")
	}
	fmt.Println("")
}

//Draw 将pic绘制到目标
func (gr *Ground) Draw(m pixel.Matrix) {
	if gr.sprite == nil {
		return
	}
	for i := 0.0; i <= gr.bound.W(); i += gr.pic.Bounds().W() {
		for j := 0.0; j <= gr.bound.H(); j += gr.pic.Bounds().H() {
			gr.sprite.Draw(gr.win, m.Moved(pixel.V(i, j)))
		}
	}
}

//Draw 将pic绘制到目标
func (wls *Walls) Draw(m pixel.Matrix) {
	for i := 0; i < latticeNum*2; i++ {
		if isEnclosure(i) || wls.wall[i] == false {
			continue
		}
		//var pos pixel.Vec
		if i%2 == 0 {
			pos := pixel.V(float64(lattice(i)%defaultN)*defaultDis, float64(lattice(i)/defaultN+1)*defaultDis-wls.pic.Bounds().H()/2)
			wls.sprite.Draw(wls.win, m.Moved(pos))
		} else {
			pos := pixel.V(float64(lattice(i)%defaultN+1)*defaultDis+wls.pic.Bounds().H()/2, float64(lattice(i)/defaultN)*defaultDis)
			wls.sprite.Draw(wls.win, m.Moved(pos).Rotated(pos, math.Pi/2))
		}
	}
}

//loadPicture 加载素材
func loadPicture(path string) (pixel.Picture, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}
	return pixel.PictureDataFromImage(img), nil
}

func loadAnimationSheet(sheetPath, descPath string, frameWidth float64) (sheet pixel.Picture, anims map[string][]pixel.Rect, err error) {
	// total hack, nicely format the error at the end, so I don't have to type it every time
	defer func() {
		if err != nil {
			err = errors.Wrap(err, "error loading animation sheet")
		}
	}()

	// open and load the spritesheet
	sheetFile, err := os.Open(sheetPath)
	if err != nil {
		return nil, nil, err
	}
	defer sheetFile.Close()
	sheetImg, _, err := image.Decode(sheetFile)
	if err != nil {
		return nil, nil, err
	}
	sheet = pixel.PictureDataFromImage(sheetImg)

	// create a slice of frames inside the spritesheet
	var frames []pixel.Rect
	for x := 0.0; x+frameWidth <= sheet.Bounds().Max.X; x += frameWidth {
		frames = append(frames, pixel.R(
			x,
			0,
			x+frameWidth,
			sheet.Bounds().H(),
		))
	}

	descFile, err := os.Open(descPath)
	if err != nil {
		return nil, nil, err
	}
	defer descFile.Close()

	anims = make(map[string][]pixel.Rect)

	// load the animation information, name and interval inside the spritesheet
	desc := csv.NewReader(descFile)
	for {
		anim, err := desc.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}

		name := anim[0]
		start, _ := strconv.Atoi(anim[1])
		end, _ := strconv.Atoi(anim[2])

		anims[name] = frames[start : end+1]
	}

	return sheet, anims, nil
}

func run() {
	cfg := pixelgl.WindowConfig{
		Title:  "迷宫游戏",
		Bounds: defaultBound,
		VSync:  true,
	}
	win, err := pixelgl.NewWindow(cfg)
	if err != nil {
		panic(err)
	}

	canvas := pixelgl.NewCanvas(pixel.R(-1024/2, -1024/2, 1024/2, 1024/2))
	//canvas := pixelgl.NewCanvas(pixel.R(-256, -256, 256, 256))

	sheet, anims, err := loadAnimationSheet("sheet.png", "sheet.csv", 12*3)
	if err != nil {
		panic(err)
	}

	picGrs, err := loadPicture("grass.png")
	if err != nil {
		panic(err)
	}
	ground := Ground{
		pic:   picGrs,
		bound: defaultBound,
		win:   win,
	}
	ground.Init()

	goal := Goal{
		pos:    pixel.V((float64(defaultN)-0.5)*defaultDis, (float64(defaultN)-0.5)*defaultDis),
		radius: 30.0,
	}
	goal.Init()

	picWl, err := loadPicture("bricks.png")
	if err != nil {
		panic(err)
	}
	walls := Walls{
		pic:   picWl,
		bound: defaultBound,
		win:   win,
	}
	walls.Init()
	walls.SetRandWalls()
	t1 := time.Now()
	walls.MakeMazeEvenly()
	fmt.Printf("%v\n", time.Since(t1))
	printMaze(&(walls.wall))

	player := Player{
		wls:     &walls,
		canvas:  canvas,
		pic:     sheet,
		anims:   anims,
		vec:     pixel.V(12*3/2.0, 14*3/2.0),
		pos:     pixel.V(0, 0),
		counter: 0.0,
		state:   idle,
		dire:    stand,
		speed:   64,
		change:  idle,
	}
	player.frame = pixel.R(0, 0, 12*3, 14*3)
	imd := imdraw.New(nil)
	imd.Precision = 32

	last := time.Now()
	for !win.Closed() {
		dt := time.Since(last).Seconds()
		last = time.Now()

		win.Clear(colornames.Skyblue)
		ground.Draw(pixel.IM.Moved(ground.win.Bounds().Center()))
		walls.Draw(pixel.IM.Moved(walls.win.Bounds().Center()))
		imd.Clear()
		goal.Draw(imd)
		imd.Draw(win)

		player.dire = stand
		player.change = idle
		if win.Pressed(pixelgl.KeyLeft) {
			//fmt.Println("left")
			player.dire = left
			player.change = running
		}
		if win.Pressed(pixelgl.KeyRight) {
			//fmt.Println("right")
			player.dire = right
			player.change = running
		}
		if win.Pressed(pixelgl.KeyUp) {
			//fmt.Println("up")
			player.dire = up
			player.change = uping
		}
		if win.Pressed(pixelgl.KeyDown) {
			//fmt.Println("down")
			player.dire = down
			player.change = jumping
		}
		canvas.Clear(color.RGBA{0x00, 0x00, 0x00, 0x00})

		player.update(dt)
		player.Draw(canvas)
		win.SetMatrix(pixel.IM)
		//canvas.Draw(win, pixel.IM.Moved(win.Bounds().Center()))
		canvas.Draw(win, pixel.IM.Moved(player.pos))
		win.Update()
	}
}

func main() {
	pixelgl.Run(run)
}
