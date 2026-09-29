import {staticFile} from 'remotion';

// The palette from brand/README.md: the UI's own colours.
export const color = {
  ink: '#171717',
  paper: '#ececec',
  violet: '#7c5cff',
  mint: '#2dd4bf',
  amber: '#facc15',
  green: '#4ade80',
  smoke: '#8a8a8a',
  line: '#2e2e2e',
};

export const font = {
  mono: "'JetBrains Mono', ui-monospace, monospace",
  sans: "'Inter', system-ui, sans-serif",
};

export const fontFaces = `
@font-face { font-family: 'JetBrains Mono'; font-weight: 400; src: url(${staticFile('fonts/JetBrainsMono-Regular.ttf')}); }
@font-face { font-family: 'JetBrains Mono'; font-weight: 700; src: url(${staticFile('fonts/JetBrainsMono-Bold.ttf')}); }
@font-face { font-family: 'JetBrains Mono'; font-weight: 800; src: url(${staticFile('fonts/JetBrainsMono-ExtraBold.ttf')}); }
@font-face { font-family: 'Inter'; font-weight: 400; src: url(${staticFile('fonts/Inter-Regular.ttf')}); }
@font-face { font-family: 'Inter'; font-weight: 600; src: url(${staticFile('fonts/Inter-SemiBold.ttf')}); }
@font-face { font-family: 'Inter'; font-weight: 700; src: url(${staticFile('fonts/Inter-Bold.ttf')}); }
`;

// The music, Digital Clouds (Mixkit), is 163.5 BPM: a beat is 11.0 frames at 30 fps, a bar 44.
// Its drop is 4.53 s in; MUSIC_START skips into the track so the drop lands when the intro ends.
export const FPS = 30;
export const BPM = 163.5;
export const BEAT = (60 / BPM) * FPS;
export const BAR = BEAT * 4;
export const MUSIC_DROP = 4.53;
