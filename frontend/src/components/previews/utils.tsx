import React from 'react';

/**
 * Format seconds into mm:ss or hh:mm:ss format
 */
export const formatTime = (seconds: number): string => {
  if (isNaN(seconds) || seconds < 0) return '00:00';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  const pad = (n: number) => n.toString().padStart(2, '0');
  if (h > 0) {
    return `${h}:${pad(m)}:${pad(s)}`;
  }
  return `${pad(m)}:${pad(s)}`;
};

/**
 * Calculate the buffered percentage for HTMLMediaElement
 */
export const getBufferedPercent = (el: HTMLMediaElement): number => {
  if (!el.buffered || el.buffered.length === 0 || !el.duration) return 0;
  try {
    const end = el.buffered.end(el.buffered.length - 1);
    return Math.min(100, Math.max(0, (end / el.duration) * 100));
  } catch {
    return 0;
  }
};

/**
 * Syntax tokenizer for code preview lines
 */
export const renderHighlightedLine = (line: string): React.ReactNode => {
  if (!line) return <span>&nbsp;</span>;

  const parts: React.ReactNode[] = [];
  const tokenRegex =
    /(\/\/.*|#.*|\/\*[\s\S]*?\*\/)|("(?:\\.|[^"\\])*"|'[^']*'|`[^`]*`)|(\b(?:def|func|function|class|return|if|else|elif|import|from|export|default|const|let|var|type|interface|struct|package|for|range|while|switch|case|break|continue|try|except|catch|finally|async|await|select|insert|update|delete|where)\b)|(\b(?:true|false|null|nil|None|True|False|undefined)\b)|(\b\d+\b)/g;

  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = tokenRegex.exec(line)) !== null) {
    if (match.index > lastIndex) {
      parts.push(line.substring(lastIndex, match.index));
    }
    const [full, comment, str, kw, boolVal, num] = match;
    if (comment) {
      parts.push(<span key={match.index} className="token-comment">{full}</span>);
    } else if (str) {
      parts.push(<span key={match.index} className="token-string">{full}</span>);
    } else if (kw) {
      parts.push(<span key={match.index} className="token-keyword">{full}</span>);
    } else if (boolVal) {
      parts.push(<span key={match.index} className="token-boolean">{full}</span>);
    } else if (num) {
      parts.push(<span key={match.index} className="token-number">{full}</span>);
    }
    lastIndex = tokenRegex.lastIndex;
  }

  if (lastIndex < line.length) {
    parts.push(line.substring(lastIndex));
  }

  return <>{parts}</>;
};
