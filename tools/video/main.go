// Command video renders the demo video from the frames TestE2EVideo records (mise run video).
//
// It turns every .ansi frame into a PNG with freeze, then has ffmpeg play them for their recorded durations
// under a caption, between a title card and an end card, and writes demo.mp4 and demo.gif.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	pngimage "image/png"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type frame struct {
	File    string  `json:"file"`
	Millis  int     `json:"ms"`
	Caption string  `json:"caption"`
	Pointer *[2]int `json:"pointer"`
}

const (
	width, height = 1920, 1080
	uiWidth       = 1840 // the terminal, scaled to leave room for the caption
	background    = "0x171717"
	accent        = "0x7c5cff"
	fontSans      = "/usr/share/fonts/noto/NotoSans-Regular.ttf"
	fontSansBold  = "/usr/share/fonts/noto/NotoSans-Bold.ttf"
	fontMonoBold  = "/usr/share/fonts/noto/NotoSansMono-Bold.ttf"
	fontMono      = "/usr/share/fonts/noto/NotoSansMono-Regular.ttf"
)

func main() {
	frames := flag.String("frames", "", "directory with frames.json and the .ansi frames")
	out := flag.String("out", "demo", "output name, without extension")
	flag.Parse()
	if *frames == "" {
		log.Fatal("-frames is required")
	}
	if err := render(*frames, *out); err != nil {
		log.Fatal(err)
	}
}

func render(dir, out string) error {
	data, err := os.ReadFile(filepath.Join(dir, "frames.json"))
	if err != nil {
		return err
	}
	var frames []frame
	if err := json.Unmarshal(data, &frames); err != nil {
		return err
	}
	if err := freezeAll(dir, frames); err != nil {
		return err
	}

	// The concat demuxer plays each PNG for its duration; the last one is repeated, as ffmpeg's docs ask.
	var list strings.Builder
	for _, f := range frames {
		fmt.Fprintf(&list, "file '%s'\nduration %.3f\n", png(f.File), float64(f.Millis)/1000)
	}
	fmt.Fprintf(&list, "file '%s'\n", png(frames[len(frames)-1].File))
	listPath := filepath.Join(dir, "frames.txt")
	if err := os.WriteFile(listPath, []byte(list.String()), 0o644); err != nil {
		return err
	}

	// The captions are drawn on the padded frame, each for the time its frames cover.
	filters := []string{
		fmt.Sprintf("scale=%d:-2:flags=lanczos", uiWidth),
		fmt.Sprintf("pad=%d:%d:(ow-iw)/2:28:color=%s", width, height, background),
	}
	t := 0.0
	for i := 0; i < len(frames); {
		j := i
		span := 0.0
		for j < len(frames) && frames[j].Caption == frames[i].Caption {
			span += float64(frames[j].Millis) / 1000
			j++
		}
		if c := frames[i].Caption; c != "" {
			filters = append(filters, drawtext(c, fontSans, 34, "0xe6e6e6", "(w-text_w)/2", "h-72",
				fmt.Sprintf("enable='between(t,%.3f,%.3f)':alpha='min(1,(t-%.3f)/0.25)'", t, t+span, t)))
		}
		t += span
		i = j
	}
	filters = append(filters, "fps=30", "format=yuv420p")

	tmp := func(name string) string { return filepath.Join(dir, name) }
	if err := ffmpeg("-f", "concat", "-safe", "0", "-i", listPath, "-vf", strings.Join(filters, ","),
		"-c:v", "libx264", "-preset", "slow", "-crf", "18", tmp("body.mp4")); err != nil {
		return err
	}
	if err := card(tmp("title.mp4"), 3.2, []string{
		drawtext("buti", fontMonoBold, 150, "0xffffff", "(w-text_w)/2", "h/2-150", ""),
		drawtext("GitButler, in your terminal", fontSans, 48, "0xbbbbbb", "(w-text_w)/2", "h/2+40", ""),
		drawtext("commit · squash · move · stack · review", fontSans, 30, accent, "(w-text_w)/2", "h/2+130", ""),
	}); err != nil {
		return err
	}
	if err := card(tmp("end.mp4"), 4.5, []string{
		drawtext("buti", fontMonoBold, 110, "0xffffff", "(w-text_w)/2", "h/2-230", ""),
		drawtext("curl -fsSL https://raw.githubusercontent.com/BartInTheField/buti/main/install.sh | sh",
			fontMono, 30, "0xe6e6e6", "(w-text_w)/2", "h/2-40", ""),
		drawtext("or:  go install github.com/bartinthefield/buti/cmd/buti@latest", fontMono, 30, "0x999999",
			"(w-text_w)/2", "h/2+20", ""),
		drawtext("github.com/BartInTheField/buti", fontSansBold, 40, accent, "(w-text_w)/2", "h/2+130", ""),
		drawtext("MIT licensed · needs the GitButler CLI (but)", fontSans, 26, "0x777777", "(w-text_w)/2", "h/2+200", ""),
	}); err != nil {
		return err
	}

	// Cross-fade the cards into the body.
	body, err := duration(tmp("body.mp4"))
	if err != nil {
		return err
	}
	const fade = 0.6
	graph := fmt.Sprintf("[0:v][1:v]xfade=transition=fade:duration=%.1f:offset=%.1f[a];"+
		"[a][2:v]xfade=transition=fade:duration=%.1f:offset=%.3f,format=yuv420p[v]",
		fade, 3.2-fade, fade, 3.2-fade+body-fade)
	if err := ffmpeg("-i", tmp("title.mp4"), "-i", tmp("body.mp4"), "-i", tmp("end.mp4"),
		"-filter_complex", graph, "-map", "[v]", "-c:v", "libx264", "-preset", "slow", "-crf", "18",
		"-movflags", "+faststart", out+".mp4"); err != nil {
		return err
	}
	// A GIF for the README and social posts: half size, 12 fps, a palette per scene.
	return ffmpeg("-i", out+".mp4", "-vf",
		"fps=12,scale=960:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128:stats_mode=diff[p];"+
			"[s1][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle", out+".gif")
}

// freezeAll renders the frames to PNG, a few at a time.
func freezeAll(dir string, frames []frame) error {
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	errs := make(chan error, len(frames))
	for _, f := range frames {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			src := filepath.Join(dir, f.File)
			dst := filepath.Join(dir, png(f.File))
			if _, err := os.Stat(dst); err == nil {
				return
			}
			cmd := exec.Command("freeze", "--language", "ansi", "--window=false", "--padding", "20",
				"--font.size", "14", "--output", dst, src)
			if out, err := cmd.CombinedOutput(); err != nil {
				errs <- fmt.Errorf("freeze %s: %v\n%s", f.File, err, out)
				return
			}
			if f.Pointer != nil {
				if err := drawPointer(dst, f.Pointer[0], f.Pointer[1]); err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		return err
	}
	return nil
}

// drawPointer paints a mouse pointer on a frame at a terminal cell. freeze lays the text out at font size 14 with
// a line height of 1.2 and a padding of 20, and renders the PNG at four times that: cells are 33.6 by 67.2
// pixels, 80 pixels in from the top left.
func drawPointer(path string, col, row int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	src, err := pngimage.Decode(f)
	f.Close()
	if err != nil {
		return err
	}
	img := image.NewRGBA(src.Bounds())
	draw.Draw(img, img.Bounds(), src, image.Point{}, draw.Src)
	ox := 80 + (float64(col)+0.5)*33.6
	oy := 80 + (float64(row)+0.5)*67.2
	// A classic arrow, 22 units tall, in the terminal's units.
	shape := [][2]float64{{0, 0}, {0, 17}, {4.5, 13}, {7.5, 19.5}, {10.5, 18}, {7.5, 12}, {13, 12}}
	const scale = 3.6
	fill := func(inset float64, c color.RGBA) {
		var pts [][2]float64
		for _, p := range shape {
			pts = append(pts, [2]float64{ox + p[0]*scale, oy + p[1]*scale})
		}
		for y := int(oy) - 4; y < int(oy+22*scale)+4; y++ {
			for x := int(ox) - 4; x < int(ox+14*scale)+4; x++ {
				if inPolygon(float64(x)+0.5, float64(y)+0.5, pts, inset) {
					img.SetRGBA(x, y, c)
				}
			}
		}
	}
	fill(0, color.RGBA{0, 0, 0, 255})
	fill(3, color.RGBA{255, 255, 255, 255})
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return pngimage.Encode(out, img)
}

// inPolygon reports whether the point is inside the polygon, at least inset away from every edge.
func inPolygon(x, y float64, pts [][2]float64, inset float64) bool {
	inside := false
	for i, j := 0, len(pts)-1; i < len(pts); j, i = i, i+1 {
		xi, yi, xj, yj := pts[i][0], pts[i][1], pts[j][0], pts[j][1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			inside = !inside
		}
		if inset > 0 {
			// Distance from the point to the edge segment.
			dx, dy := xj-xi, yj-yi
			t := ((x-xi)*dx + (y-yi)*dy) / (dx*dx + dy*dy)
			t = math.Max(0, math.Min(1, t))
			ex, ey := xi+t*dx-x, yi+t*dy-y
			if math.Sqrt(ex*ex+ey*ey) < inset {
				return false
			}
		}
	}
	return inside
}

func png(ansi string) string { return strings.TrimSuffix(ansi, ".ansi") + ".png" }

func drawtext(text, font string, size int, color, x, y, extra string) string {
	esc := strings.NewReplacer(`\`, `\\`, `'`, `'\''`, `:`, `\:`, `%`, `\%`).Replace(text)
	s := fmt.Sprintf("drawtext=fontfile=%s:text='%s':fontsize=%d:fontcolor=%s:x=%s:y=%s", font, esc, size, color, x, y)
	if extra != "" {
		s += ":" + extra
	}
	return s
}

// card writes a still of the given texts on the background.
func card(out string, seconds float64, texts []string) error {
	return ffmpeg("-f", "lavfi", "-i", fmt.Sprintf("color=c=%s:s=%dx%d:r=30:d=%.1f", background, width, height, seconds),
		"-vf", strings.Join(texts, ",")+",format=yuv420p", "-c:v", "libx264", "-preset", "slow", "-crf", "18", out)
}

func duration(path string) (float64, error) {
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path).Output()
	if err != nil {
		return 0, err
	}
	var d float64
	_, err = fmt.Sscan(strings.TrimSpace(string(out)), &d)
	return d, err
}

func ffmpeg(args ...string) error {
	cmd := exec.Command("ffmpeg", append([]string{"-y", "-hide_banner", "-loglevel", "error"}, args...)...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
