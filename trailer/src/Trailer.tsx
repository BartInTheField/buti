import React from 'react';
import {AbsoluteFill, Audio, Sequence, interpolate, staticFile, useCurrentFrame} from 'remotion';
import {BAR, FPS, MUSIC_DROP, color, fontFaces} from './brand';
import {Intro, Outro} from './Cards';
import {Label} from './Overlays';
import {Screen} from './Screen';
import {shotsDuration} from './shots';

export const INTRO = BAR * 2;
export const OUTRO = BAR * 4;
export const TOTAL = Math.round(INTRO + shotsDuration + OUTRO);

export const Trailer: React.FC = () => {
  const frame = useCurrentFrame();
  const volume = interpolate(frame, [0, 6, TOTAL - 45, TOTAL], [0, 1, 1, 0], {extrapolateRight: 'clamp'});
  return (
    <AbsoluteFill style={{backgroundColor: color.ink}}>
      <style>{fontFaces}</style>
      <Audio src={staticFile('music/digital-clouds.mp3')} startFrom={Math.round(MUSIC_DROP * FPS - INTRO)} volume={volume} />
      <Sequence from={0} durationInFrames={Math.round(INTRO)}>
        <Intro duration={Math.round(INTRO)} />
      </Sequence>
      <Sequence from={Math.round(INTRO)} durationInFrames={Math.round(shotsDuration)}>
        <Screen />
        <Label />
      </Sequence>
      <Sequence from={Math.round(INTRO + shotsDuration)} durationInFrames={Math.round(OUTRO)}>
        <Outro duration={Math.round(OUTRO)} />
      </Sequence>
    </AbsoluteFill>
  );
};
