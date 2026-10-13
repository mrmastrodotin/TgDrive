import React, { useState, useMemo } from 'react';
import { Search, WrapText, X } from 'lucide-react';
import { renderHighlightedLine } from './utils';

export interface TextCodePreviewProps {
  textContent: string;
  loading: boolean;
}

export const TextCodePreview: React.FC<TextCodePreviewProps> = ({
  textContent,
  loading,
}) => {
  const [searchQuery, setSearchQuery] = useState('');
  const [wordWrap, setWordWrap] = useState(true);

  const codeLines = useMemo(() => {
    return textContent ? textContent.split('\n') : [];
  }, [textContent]);

  const filteredCodeLines = useMemo(() => {
    if (!searchQuery) return codeLines;
    return codeLines.map((line) => {
      if (!line.toLowerCase().includes(searchQuery.toLowerCase())) return line;
      return line;
    });
  }, [codeLines, searchQuery]);

  return (
    <div className="preview-code-viewport">
      <div className="canvas-sub-header">
        <div className="search-in-code-box">
          <Search size={14} className="search-icon" />
          <input
            type="text"
            placeholder="Find in file..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="search-input-field"
          />
          {searchQuery && (
            <button className="clear-search-btn" onClick={() => setSearchQuery('')}>
              <X size={12} />
            </button>
          )}
        </div>

        <div className="canvas-actions-right">
          <button
            className={`wrap-toggle-btn ${wordWrap ? 'active' : ''}`}
            onClick={() => setWordWrap(!wordWrap)}
            title="Toggle Word Wrap"
          >
            <WrapText size={14} />
            <span>{wordWrap ? 'Wrap On' : 'Wrap Off'}</span>
          </button>
          <div className="canvas-stats">
            <span>{filteredCodeLines.length} lines</span>
          </div>
        </div>
      </div>

      {loading ? (
        <div className="preview-loading-state">
          <span className="spinner-border"></span>
          <p>Decrypting code from Telegram...</p>
        </div>
      ) : (
        <div className={`raw-code-canvas ${wordWrap ? 'wrap-lines' : 'no-wrap'}`}>
          <pre className="code-pre-block">
            {filteredCodeLines.map((line, idx) => {
              const isMatch = searchQuery && line.toLowerCase().includes(searchQuery.toLowerCase());
              return (
                <div key={idx} className={`code-line-row ${isMatch ? 'highlight-match' : ''}`}>
                  <span className="line-num">{idx + 1}</span>
                  <span className="line-content">{renderHighlightedLine(line)}</span>
                </div>
              );
            })}
          </pre>
        </div>
      )}
    </div>
  );
};
