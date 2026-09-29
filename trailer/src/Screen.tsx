import React from 'react';
import {AbsoluteFill, Img, interpolate, staticFile, useCurrentFrame} from 'remotion';
import type {Cell, Shot} from './shots';
import {shots, shotStart} from './shots';
import {color} from './brand';

// freeze renders the 160x40 terminal at font size 14, line height 1.2 and padding 20, four times over: the PNG is
// 5527x2952 with 33.6x67.2 pixel cells that start 80 pixels in.
const IMG_W = 5527;
const IMG_H = 2952;
const CELL_W = 33.6;
const CELL_H = 67.2;
const PAD = 80;

const W = 1920;
const H = 1080;
const K = W / IMG_W;
const TOP = (H - IMG_H * K) / 2;

type Camera = {x: number; y: number; zoom: number};

const cameraOf = (s: Shot): Camera => {
  const c: Cell = s.focus ?? {col: 80, row: 20};
  return {
    x: (PAD + (c.col + 0.5) * CELL_W) * K,
    y: TOP + (PAD + (c.row + 0.5) * CELL_H) * K,
    zoom: s.zoom ?? 1,
  };
};

const smooth = (t: number) => t * t * (3 - 2 * t);

// sceneStart is the index of the first shot of the scene shot i belongs to.
const sceneStart = (i: number) => {
  let j = i;
  while (j > 0 && shots[j - 1].scene === shots[i].scene) {
    j--;
  }
  return j;
};

// camera returns where the camera is at a frame of a shot. It starts a scene on the shot's target and, within a
// scene, glides from wherever it actually was when the previous shot ended, so a run of short shots (typing,
// a drag) reads as one continuous move instead of a restart every shot. A slow push-in over the whole scene keeps
// the picture alive without a still ever jumping.
const GLIDE = 10;
const camera = (i: number, local: number): Camera => {
  const to = cameraOf(shots[i]);
  const from = i > 0 && shots[i - 1].scene === shots[i].scene ? camera(i - 1, shots[i - 1].dur) : to;
  const t = smooth(Math.min(1, local / GLIDE));
  return {
    x: from.x + (to.x - from.x) * t,
    y: from.y + (to.y - from.y) * t,
    zoom: from.zoom + (to.zoom - from.zoom) * t,
  };
};

const push = (i: number, local: number) => {
  const elapsed = shotStart(i) - shotStart(sceneStart(i)) + local;
  return 1 + Math.min(0.04, elapsed * 0.0006);
};

export const findShot = (frame: number): {i: number; local: number} | null => {
  let t = 0;
  for (let i = 0; i < shots.length; i++) {
    if (frame < t + shots[i].dur) {
      return {i, local: frame - t};
    }
    t += shots[i].dur;
  }
  return null;
};

export const Screen: React.FC = () => {
  const frame = useCurrentFrame();
  const at = findShot(frame);
  if (!at) {
    return null;
  }
  const s = shots[at.i];
  const cam = camera(at.i, at.local);
  // Keep the picture covering the canvas.
  const z = cam.zoom * push(at.i, at.local);
  const tx = Math.min(0, Math.max(W - W * z, W / 2 - cam.x * z));
  const ty = Math.min(TOP * z, Math.max(H - (TOP + IMG_H * K) * z, H / 2 - cam.y * z));
  // A quick flash on a cut between scenes, so the eye registers it.
  const prev = shots[at.i - 1];
  const cut = !prev || prev.scene !== s.scene;
  const flash = cut ? interpolate(at.local, [0, 4], [0.18, 0], {extrapolateRight: 'clamp'}) : 0;
  return (
    <AbsoluteFill style={{backgroundColor: color.ink, overflow: 'hidden'}}>
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          width: IMG_W,
          height: IMG_H,
          // The PNG is laid out at its native size and scaled down here, so a zoomed-in shot samples the full
          // resolution instead of magnifying a 1920-wide copy.
          transform: `translate(${tx}px, ${ty}px) scale(${z * K})`,
          transformOrigin: '0 0',
        }}
      >
        <Img src={staticFile(`frames/${(shots[at.i].frame + '').padStart(4, '0')}.png`)} style={{width: IMG_W, height: IMG_H, display: 'block'}} />
      </div>
      <AbsoluteFill style={{backgroundColor: color.violet, opacity: flash, pointerEvents: 'none'}} />
      {/* A soft vignette keeps the eye on the centre when zoomed in. */}
      <AbsoluteFill
        style={{
          background: 'radial-gradient(ellipse at center, rgba(23,23,23,0) 55%, rgba(23,23,23,0.55) 100%)',
          pointerEvents: 'none',
        }}
      />
    </AbsoluteFill>
  );
};

// shotAt is the shot on screen at a video frame, with the label and key that apply to it.
export const shotAt = (frame: number): {shot: Shot; local: number; start: number; label?: string; labelSince: number; key?: string} | null => {
  const at = findShot(frame);
  if (!at) {
    return null;
  }
  let label: string | undefined;
  let labelSince = 0;
  for (let i = 0; i <= at.i; i++) {
    if (shots[i].label) {
      label = shots[i].label;
      labelSince = shotStart(i);
    }
  }
  return {shot: shots[at.i], local: at.local, start: shotStart(at.i), label, labelSince, key: shots[at.i].key};
};
