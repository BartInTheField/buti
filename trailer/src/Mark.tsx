import React from 'react';
import {color} from './brand';

// The brand mark from brand/mark.svg: three lanes, the middle one carrying a commit. `progress` (0..1) grows the
// lanes and pops the dot, for the intro; 1 draws it complete.
export const Mark: React.FC<{size: number; progress?: number}> = ({size, progress = 1}) => {
  const lanes = [
    {x: 60, y: 60, h: 112},
    {x: 118, y: 60, h: 136},
    {x: 176, y: 84, h: 112},
  ];
  const dot = Math.max(0, Math.min(1, (progress - 0.6) / 0.4));
  return (
    <svg width={size} height={size} viewBox="0 0 256 256">
      <rect width="256" height="256" rx="56" fill={color.violet} />
      {lanes.map((l, i) => {
        const p = Math.max(0, Math.min(1, (progress - i * 0.12) / 0.6));
        return <rect key={i} x={l.x} y={l.y} width="20" height={l.h * p} rx="10" fill={color.paper} />;
      })}
      <circle cx="128" cy="140" r={22 * dot} fill={color.mint} stroke={color.violet} strokeWidth={8 * dot} />
    </svg>
  );
};

export const Wordmark: React.FC<{size: number; color?: string}> = ({size, color: c = color.paper}) => (
  <span
    style={{
      fontFamily: "'JetBrains Mono', monospace",
      fontWeight: 800,
      fontSize: size,
      letterSpacing: '-0.04em',
      color: c,
      lineHeight: 1,
    }}
  >
    buti
  </span>
);
