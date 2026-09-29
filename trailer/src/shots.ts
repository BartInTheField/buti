import frames from './frames.json';
import {BAR, BEAT} from './brand';

// A cell on the 160x40 terminal the frames were recorded on.
export type Cell = {col: number; row: number};

export type Shot = {
  // Index into frames.json.
  frame: number;
  // How long it is on screen, in video frames.
  dur: number;
  // The scene it belongs to. The camera glides between shots of one scene and cuts between scenes.
  scene: string;
  // Where the camera looks (the centre of the terminal by default) and how far it zooms in.
  focus?: Cell;
  zoom?: number;
  // The big label for the scene; set on its first shot and kept until a shot sets another.
  label?: string;
  // A key cap that pops in when the shot starts.
  key?: string;
};

type Frame = {file: string; anchors?: Record<string, [number, number]>; pointer?: [number, number]};
const all = frames as unknown as Frame[];

// anchor looks a recorded text up on a frame; the recorder noted where each is.
const anchor = (frame: number, name: string, dcol = 0, drow = 0): Cell => {
  const a = all[frame].anchors?.[name];
  if (!a) {
    throw new Error(`frame ${frame} has no anchor ${name}`);
  }
  return {col: a[0] + dcol, row: a[1] + drow};
};
const pointer = (frame: number): Cell => {
  const p = all[frame].pointer;
  if (!p) {
    throw new Error(`frame ${frame} has no pointer`);
  }
  return {col: p[0], row: p[1]};
};

const lanes: Cell = {col: 100, row: 8};
const centre: Cell = {col: 80, row: 20};
const composer: Cell = {col: 84, row: 16};

const half = BEAT * 2;

export const shots: Shot[] = [
  // Bars 2-3: the workspace, one slow push.
  {frame: 0, dur: BAR * 2, scene: 'workspace', focus: {col: 88, row: 12}, zoom: 1.25, label: 'Your whole workspace. One screen.'},
  // Bar 4: a commit's diff, zoomed on the details pane.
  {frame: 3, dur: BAR, scene: 'diff', focus: {col: 86, row: 28}, zoom: 1.6, label: 'Every diff, highlighted.'},

  // Bars 5-8: commit. Pick the file, press c, the target hops across the lanes, type, land.
  {frame: 4, dur: BEAT, scene: 'commit', focus: lanes, zoom: 1.45, label: 'Commit.', key: 'c'},
  {frame: 6, dur: BEAT, scene: 'commit', focus: anchor(6, 'target', 0, 4), zoom: 1.6},
  {frame: 7, dur: BEAT, scene: 'commit', focus: anchor(7, 'target', 0, 4), zoom: 1.6},
  {frame: 9, dur: BEAT, scene: 'commit', focus: anchor(9, 'target', 0, 3), zoom: 1.6, key: 'enter'},
  ...Array.from({length: 21}, (_, i) => ({
    frame: 11 + i,
    dur: 2,
    scene: 'commit',
    focus: anchor(11, 'composer', 40, 10),
    zoom: 1.35,
  })),
  {frame: 31, dur: BEAT, scene: 'commit', focus: anchor(11, 'composer', 40, 10), zoom: 1.35, key: 'enter'},
  {frame: 33, dur: BAR - BEAT + 3, scene: 'commit', focus: {col: 118, row: 15}, zoom: 1.8, label: 'Landed.'},

  // Bars 9-11: squash by dragging.
  {frame: 34, dur: half, scene: 'squash', focus: pointer(34), zoom: 1.7, label: 'Or drag.'},
  ...[36, 38, 40, 41].map((f) => ({frame: f, dur: 4, scene: 'squash', focus: pointer(f), zoom: 1.7})),
  {frame: 42, dur: half, scene: 'squash', focus: pointer(42), zoom: 1.7, label: 'Squash.'},
  {frame: 43, dur: BAR, scene: 'squash', focus: anchor(43, 'composer', 40, 10), zoom: 1.35, key: 'enter'},
  {frame: 44, dur: half, scene: 'squash', focus: lanes, zoom: 1.3},

  // Bars 12-13: uncommit by dragging onto Unstaged.
  {frame: 45, dur: half, scene: 'uncommit', focus: pointer(45), zoom: 1.5, label: 'Uncommit.'},
  ...[47, 49, 51, 53, 55].map((f) => ({frame: f, dur: 4, scene: 'uncommit', focus: pointer(f), zoom: 1.5})),
  {frame: 57, dur: half - 2, scene: 'uncommit', focus: pointer(57), zoom: 1.5},
  {frame: 58, dur: BAR, scene: 'uncommit', focus: {col: 20, row: 6}, zoom: 1.4},

  // Bars 14-17: review comments and the agent.
  {frame: 59, dur: half, scene: 'review', focus: {col: 70, row: 27}, zoom: 1.6, label: 'Comment on the diff.'},
  // The composer is a box over the lanes, cols 42-126, rows 6-26.
  {frame: 60, dur: half, scene: 'review', focus: composer, zoom: 1.35},
  ...Array.from({length: 27}, (_, i) => ({
    frame: 61 + i,
    dur: 2,
    scene: 'review',
    focus: composer,
    zoom: 1.35,
  })),
  {frame: 88, dur: half, scene: 'review', focus: composer, zoom: 1.35},
  {frame: 90, dur: BAR * 1.5, scene: 'review', focus: anchor(90, 'comment', 10, -3), zoom: 1.8, label: 'Your agent picks it up.'},
  {frame: 91, dur: BAR * 2, scene: 'review', focus: anchor(91, 'resolved', 20, -3), zoom: 1.8, label: 'And closes it.'},

  // Bars 18-19: the palette.
  {frame: 92, dur: half, scene: 'palette', focus: {col: 80, row: 14}, zoom: 1.4, label: 'Every command. One palette.', key: 'ctrl+p'},
  ...[93, 94, 95, 96, 97, 98].map((f) => ({frame: f, dur: 5, scene: 'palette', focus: {col: 80, row: 14}, zoom: 1.4})),
  {frame: 99, dur: BAR * 2 - 30, scene: 'palette', focus: {col: 80, row: 14}, zoom: 1.4},

  // A bar-per-cut recap.
  {frame: 100, dur: BAR, scene: 'm1', focus: centre, zoom: 1.15, label: 'Keyboard first.'},
  {frame: 33, dur: BAR, scene: 'm2', focus: {col: 118, row: 15}, zoom: 1.6, label: 'Mouse when you want it.'},
  {frame: 57, dur: BAR, scene: 'm3', focus: pointer(57), zoom: 1.6},
  {frame: 91, dur: BAR * 1.5, scene: 'm4', focus: anchor(91, 'resolved', 20, -3), zoom: 1.6, label: 'Review your agent acts on.'},
];

export const shotStart = (i: number) => shots.slice(0, i).reduce((t, s) => t + s.dur, 0);
export const shotsDuration = shotStart(shots.length);
