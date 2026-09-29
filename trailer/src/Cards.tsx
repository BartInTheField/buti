import React from 'react';
import {AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';
import {BEAT, color, font} from './brand';
import {Mark, Wordmark} from './Mark';
import {Tagline} from './Tagline';

// Intro: the mark draws itself on the beat, the wordmark snaps in, the tagline lands.
export const Intro: React.FC<{duration: number}> = ({duration}) => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const mark = spring({frame, fps, config: {damping: 14, stiffness: 160}});
  const draw = interpolate(frame, [2, 20], [0, 1], {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'});
  const word = spring({frame: frame - BEAT * 1.5, fps, config: {damping: 16, stiffness: 260, mass: 0.6}});
  const reveal = [3, 4.5, 6].map((beat) => spring({frame: frame - BEAT * beat, fps, config: {damping: 18, stiffness: 200}}));
  const out = interpolate(frame, [duration - 8, duration], [1, 0], {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'});
  const scale = interpolate(frame, [duration - 8, duration], [1, 1.08], {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'});
  return (
    <AbsoluteFill style={{backgroundColor: color.ink, alignItems: 'center', justifyContent: 'center', opacity: out}}>
      <div style={{display: 'flex', alignItems: 'center', gap: 44, transform: `scale(${scale})`}}>
        <div style={{transform: `scale(${mark}) rotate(${(1 - mark) * -12}deg)`}}>
          <Mark size={200} progress={draw} />
        </div>
        <div style={{overflow: 'hidden'}}>
          <div style={{transform: `translateY(${(1 - word) * 120}%)`}}>
            <Wordmark size={220} />
          </div>
        </div>
      </div>
      <div style={{marginTop: 56, transform: `scale(${scale})`}}>
        <Tagline size={50} reveal={reveal} />
      </div>
    </AbsoluteFill>
  );
};

// Outro: the lockup, the install line typed out and the URL.
export const Outro: React.FC<{duration: number}> = ({duration}) => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const inn = spring({frame, fps, config: {damping: 16, stiffness: 200}});
  const install = 'curl -fsSL https://raw.githubusercontent.com/BartInTheField/buti/main/install.sh | sh';
  const typed = Math.floor(interpolate(frame, [BEAT * 2, BEAT * 6], [0, install.length], {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'}));
  const url = spring({frame: frame - BEAT * 6.5, fps, config: {damping: 18, stiffness: 200}});
  const caret = Math.floor(frame / 8) % 2 === 0 && typed < install.length;
  const out = interpolate(frame, [duration - 12, duration], [1, 0], {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'});
  return (
    <AbsoluteFill style={{backgroundColor: color.ink, alignItems: 'center', justifyContent: 'center', opacity: out}}>
      <div style={{display: 'flex', alignItems: 'center', gap: 36, transform: `scale(${inn})`, opacity: inn}}>
        <Mark size={140} />
        <Wordmark size={160} />
      </div>
      <div style={{marginTop: 36, opacity: inn}}>
        <Tagline size={34} gap={0.2} />
      </div>
      <div
        style={{
          marginTop: 56,
          fontFamily: font.mono,
          fontSize: 30,
          color: color.paper,
          backgroundColor: '#1f1f1f',
          border: `2px solid ${color.line}`,
          borderRadius: 14,
          padding: '20px 32px',
          minWidth: 1500,
          textAlign: 'left',
          opacity: interpolate(frame, [BEAT * 1.5, BEAT * 2], [0, 1], {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'}),
        }}
      >
        <span style={{color: color.mint}}>$ </span>
        {install.slice(0, typed)}
        <span style={{opacity: caret ? 1 : 0, color: color.violet}}>▍</span>
      </div>
      <div
        style={{
          marginTop: 44,
          fontFamily: font.sans,
          fontWeight: 700,
          fontSize: 44,
          color: color.violet,
          opacity: url,
          transform: `translateY(${(1 - url) * 20}px)`,
        }}
      >
        github.com/BartInTheField/buti
      </div>
      <div
        style={{
          position: 'absolute',
          bottom: 28,
          fontFamily: font.sans,
          fontSize: 20,
          color: '#5a5a5a',
          opacity: url,
        }}
      >
        Free and open source · MIT
      </div>
    </AbsoluteFill>
  );
};
