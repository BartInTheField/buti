import React from 'react';
import {AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';
import {color, font} from './brand';
import {shotAt} from './Screen';

// Label is the scene's big line, bottom left, sliding up behind a violet bar when it changes.
export const Label: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const at = shotAt(frame);
  if (!at || !at.label) {
    return null;
  }
  const since = frame - at.labelSince;
  const enter = spring({frame: since, fps, config: {damping: 18, stiffness: 220, mass: 0.7}});
  const y = interpolate(enter, [0, 1], [60, 0]);
  return (
    <AbsoluteFill style={{pointerEvents: 'none'}}>
      <div
        style={{
          position: 'absolute',
          left: 0,
          right: 0,
          bottom: 0,
          height: 340,
          background: 'linear-gradient(to top, rgba(23,23,23,0.96) 0%, rgba(23,23,23,0.75) 45%, rgba(23,23,23,0) 100%)',
          opacity: enter,
        }}
      />
      <div
        style={{
          position: 'absolute',
          left: 72,
          bottom: 64,
          display: 'flex',
          alignItems: 'stretch',
          gap: 22,
          transform: `translateY(${y}px)`,
          opacity: enter,
        }}
      >
        <div style={{width: 12, borderRadius: 6, backgroundColor: color.violet}} />
        <div
          style={{
            fontFamily: font.sans,
            fontWeight: 700,
            fontSize: 76,
            letterSpacing: '-0.03em',
            lineHeight: 1.05,
            color: color.paper,
            textShadow: '0 4px 24px rgba(0,0,0,0.6)',
          }}
        >
          {at.label}
        </div>
      </div>
    </AbsoluteFill>
  );
};

// KeyCap pops the key a shot presses in the top right and lets it fade. Not in the cut at the moment: it drew
// the eye away from the screen. Add it back to Trailer.tsx to show the keys.
export const KeyCap: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const at = shotAt(frame);
  if (!at || !at.key) {
    return null;
  }
  const local = frame - at.start;
  const pop = spring({frame: local, fps, config: {damping: 12, stiffness: 300, mass: 0.5}});
  const fade = interpolate(local, [0, 14, 22], [1, 1, 0], {extrapolateRight: 'clamp'});
  const keys = at.key.split('+');
  return (
    <AbsoluteFill style={{pointerEvents: 'none'}}>
      <div
        style={{
          position: 'absolute',
          right: 72,
          top: 56,
          display: 'flex',
          gap: 14,
          alignItems: 'center',
          transform: `scale(${0.6 + 0.4 * pop})`,
          transformOrigin: 'top right',
          opacity: Math.min(pop, fade),
        }}
      >
        {keys.map((k, i) => (
          <React.Fragment key={k}>
            {i > 0 && <span style={{color: color.smoke, fontFamily: font.sans, fontSize: 40}}>+</span>}
            <div
              style={{
                fontFamily: font.mono,
                fontWeight: 700,
                fontSize: 54,
                color: color.ink,
                backgroundColor: color.paper,
                borderRadius: 14,
                padding: '10px 26px',
                boxShadow: `0 8px 0 ${color.violet}, 0 16px 40px rgba(0,0,0,0.6)`,
                minWidth: 44,
                textAlign: 'center',
              }}
            >
              {k}
            </div>
          </React.Fragment>
        ))}
      </div>
    </AbsoluteFill>
  );
};
