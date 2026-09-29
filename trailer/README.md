# buti trailer

A [Remotion](https://www.remotion.dev) project that cuts the marketing trailer from the frames `TestE2EVideo` records
(`mise run video`), with the brand from `../brand`.

```sh
mise run trailer        # renders out/trailer.mp4, out/banner.png, out/avatar.png
npm run studio          # opens the Remotion studio to scrub through it
```

- `src/shots.ts`: the cut. One entry per shot: which recorded frame, how long (in video frames; the music is 163.5 BPM,
  so a beat is 11 frames and a bar 44), which scene it belongs to (the camera glides within a scene and cuts
  between), where the camera looks (a cell, or an anchor the recorder noted), the zoom, the label and the key cap.
- `src/Screen.tsx`: the camera over the terminal frames.
- `src/Overlays.tsx`: the labels (and key caps, currently left out of the cut). `src/Tagline.tsx`: the tagline. `src/Cards.tsx`: the intro and outro.
- `src/Mark.tsx`, `src/brand.ts`: the mark, wordmark, colours and fonts.
- `public/music/digital-clouds.mp3`: Digital Clouds from [Mixkit](https://mixkit.co/free-stock-music/), under the
  Mixkit Stock Music Free License: free for commercial use and social media, no credit needed. The tempo and the
  drop are in `src/brand.ts`; change both when swapping the track.
