import React, { useState, useMemo } from 'react';
import { marked } from 'marked';
import { Sparkles, Code2 } from 'lucide-react';
import { renderHighlightedLine } from './utils';

export interface MarkdownPreviewProps {
  textContent: string;
  loading: boolean;
}

export const MarkdownPreview: React.FC<MarkdownPreviewProps> = ({
  textContent,
  loading,
}) => {
  const [mdMode, setMdMode] = useState<'rendered' | 'raw'>('rendered');

  const renderedMarkdown = useMemo(() => {
    if (!textContent) return '';
    try {
      return marked.parse(textContent, { gfm: true, breaks: true }) as string;
    } catch (err) {
      return `<p class="error-text">Error rendering markdown: ${String(err)}</p>`;
    }
  }, [textContent]);

  const codeLines = useMemo(() => {
    return textContent ? textContent.split('\n') : [];
  }, [textContent]);

  const wordCount = useMemo(() => {
    return textContent ? textContent.split(/\s+/).filter(Boolean).length : 0;
  }, [textContent]);

  return (
    <div className="preview-markdown-viewport">
      <div className="canvas-sub-header">
        <div className="canvas-mode-toggle">
          <button
            className={`mode-tab-btn ${mdMode === 'rendered' ? 'active' : ''}`}
            onClick={() => setMdMode('rendered')}
          >
            <Sparkles size={14} />
            <span>Preview</span>
          </button>
          <button
            className={`mode-tab-btn ${mdMode === 'raw' ? 'active' : ''}`}
            onClick={() => setMdMode('raw')}
          >
            <Code2 size={14} />
            <span>Source</span>
          </button>
        </div>

        <div className="canvas-stats">
          <span>{wordCount} words</span>
          <span>•</span>
          <span>{codeLines.length} lines</span>
        </div>
      </div>

      {loading ? (
        <div className="preview-loading-state">
          <span className="spinner-border"></span>
          <p>Decrypting and rendering markdown canvas...</p>
        </div>
      ) : mdMode === 'rendered' ? (
        <div
          className="chatgpt-markdown-body"
          dangerouslySetInnerHTML={{ __html: renderedMarkdown }}
        />
      ) : (
        <div className="raw-code-canvas">
          <pre className="code-pre-block">
            {codeLines.map((line, idx) => (
              <div key={idx} className="code-line-row">
                <span className="line-num">{idx + 1}</span>
                <span className="line-content">{renderHighlightedLine(line)}</span>
              </div>
            ))}
          </pre>
        </div>
      )}
    </div>
  );
};
