import React from 'react';
import {AbsoluteFill, Composition, Still} from 'remotion';
import {FPS, color, fontFaces} from './brand';
import {Mark, Wordmark} from './Mark';
import {Tagline} from './Tagline';
import {TOTAL, Trailer} from './Trailer';

// The X header: mark, wordmark and tagline.
const Banner: React.FC = () => (
  <AbsoluteFill style={{backgroundColor: color.ink, alignItems: 'center', justifyContent: 'center'}}>
    <style>{fontFaces}</style>
    <div style={{display: 'flex', alignItems: 'center', gap: 40}}>
      <Mark size={200} />
      <div>
        <Wordmark size={200} />
        <div style={{marginTop: 16}}>
          <Tagline size={34} align="left" gap={0.15} />
        </div>
      </div>
    </div>
  </AbsoluteFill>
);

const Avatar: React.FC = () => (
  <AbsoluteFill style={{backgroundColor: color.ink, alignItems: 'center', justifyContent: 'center'}}>
    <Mark size={760} />
  </AbsoluteFill>
);

export const Root: React.FC = () => (
  <>
    <Composition id="Trailer" component={Trailer} durationInFrames={TOTAL} fps={FPS} width={1920} height={1080} />
    <Still id="Banner" component={Banner} width={1500} height={500} />
    <Still id="Avatar" component={Avatar} width={1000} height={1000} />
  </>
);
