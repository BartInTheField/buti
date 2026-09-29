import React from 'react';
import {color, font} from './brand';

// The tagline, three lines with one strong word each. `reveal` gives each line's 0..1 entrance, for the intro.
export const lines: [string, string, string][] = [
  ['', 'Parallel', ' agentic workflow'],
  ['', 'Code review', ' your agent can act on'],
  ['On top of ', 'GitButler', ''],
];

export const Tagline: React.FC<{size: number; reveal?: number[]; align?: 'center' | 'left'; gap?: number}> = ({
  size,
  reveal = [1, 1, 1],
  align = 'center',
  gap = 0.25,
}) => (
  <div style={{display: 'flex', flexDirection: 'column', alignItems: align === 'center' ? 'center' : 'flex-start', gap: size * gap}}>
    {lines.map(([pre, strong, post], i) => (
      <div
        key={i}
        style={{
          fontFamily: font.sans,
          fontWeight: 600,
          fontSize: size,
          letterSpacing: '-0.02em',
          lineHeight: 1.1,
          color: color.smoke,
          opacity: reveal[i],
          transform: `translateY(${(1 - reveal[i]) * size * 0.6}px)`,
          whiteSpace: 'nowrap',
        }}
      >
        {pre}
        <span style={{color: i === 2 ? color.mint : color.paper, fontWeight: 700}}>{strong}</span>
        {post}
      </div>
    ))}
  </div>
);
