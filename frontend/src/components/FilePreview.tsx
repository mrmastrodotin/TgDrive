import React, { useState, useEffect, useRef, useMemo } from 'react';
import {
  FileItem,
  getPreviewType,
  formatFileSize,
  PreviewableType,
} from '../types';
import { getStreamURL, getPreviewURL, downloadFile } from '../api/client';
import { marked } from 'marked';
import {
  X,
  Download,
  Copy,
  Check,
  Film,
  Music,
  FileCode,
  FileText,
  Image as ImageIcon,
  ExternalLink,
  Presentation,
  FileSpreadsheet,
  BookOpen,
  Sparkles,
  Code2,
  Table,
  ZoomIn,
  ZoomOut,
  RotateCcw,
  Maximize2,
  Search,
  WrapText,
  Play,
  Pause,
  Volume2,
  VolumeX,
  Layers,
  ShieldCheck,
} from 'lucide-react';

interface FilePreviewProps {
  file: FileItem;
  onClose: () => void;
}

export default function FilePreview({ file, onClose }: FilePreviewProps) {
  const previewType: PreviewableType = getPreviewType(file.mime_type, file.name);
  const ext = (file.name.split('.').pop() || '').toLowerCase();

  // Text / Code / Markdown state
  const [textContent, setTextContent] = useState<string>('');
  const [loadingText, setLoadingText] = useState(false);
  const [copied, setCopied] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [wordWrap, setWordWrap] = useState(true);

  // Markdown view mode ('rendered' | 'raw')
  const [mdMode, setMdMode] = useState<'rendered' | 'raw'>('rendered');

  // CSV view mode ('table' | 'raw')
  const [csvMode, setCsvMode] = useState<'table' | 'raw'>('table');

  // Image zoom state
  const [imgZoom, setImgZoom] = useState(1);
  const [imgNaturalDim, setImgNaturalDim] = useState<{ w: number; h: number } | null>(null);

  // Video state
  const videoRef = useRef<HTMLVideoElement>(null);
  const [videoPlaybackRate, setVideoPlaybackRate] = useState(1);

  // Audio state
  const audioRef = useRef<HTMLAudioElement>(null);
  const [isPlayingAudio, setIsPlayingAudio] = useState(false);
  const [audioProgress, setAudioProgress] = useState(0);
  const [audioDuration, setAudioDuration] = useState(0);
  const [audioVolume, setAudioVolume] = useState(1);
  const [isAudioMuted, setIsAudioMuted] = useState(false);

  // Keyboard close on Esc
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onClose]);

  // Fetch text/code/markdown/csv content
  useEffect(() => {
    const isTextual = ['text', 'code', 'markdown', 'csv'].includes(previewType);
    if (isTextual) {
      setLoadingText(true);
      fetch(getPreviewURL(file.id))
        .then((res) => {
          if (!res.ok) throw new Error(`HTTP ${res.status}: Failed to decrypt preview`);
          return res.text();
        })
        .then((text) => {
          setTextContent(text);
          setLoadingText(false);
        })
        .catch((err) => {
          setTextContent(`// Error loading decrypted preview: ${err.message}`);
          setLoadingText(false);
        });
    }
  }, [file.id, previewType]);

  const handleCopy = () => {
    if (!textContent) return;
    navigator.clipboard.writeText(textContent);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleDownload = async () => {
    try {
      await downloadFile(file.id, file.name);
    } catch (err) {
      console.error('Download error:', err);
    }
  };

  // Rendered Markdown HTML
  const renderedMarkdown = useMemo(() => {
    if (previewType !== 'markdown' || !textContent) return '';
    try {
      return marked.parse(textContent, { gfm: true, breaks: true }) as string;
    } catch (err) {
      return `<p class="error-text">Error rendering markdown: ${String(err)}</p>`;
    }
  }, [previewType, textContent]);

  // Parsed CSV Rows
  const parsedCSV = useMemo(() => {
    if (previewType !== 'csv' || !textContent) return { headers: [], rows: [] };
    const lines = textContent.split(/\r?\n/).filter((l) => l.trim().length > 0);
    if (lines.length === 0) return { headers: [], rows: [] };

    const parseLine = (line: string) => {
      const result: string[] = [];
      let cur = '';
      let inQuotes = false;
      for (let i = 0; i < line.length; i++) {
        const c = line[i];
        if (c === '"') {
          inQuotes = !inQuotes;
        } else if ((c === ',' || c === '\t') && !inQuotes) {
          result.push(cur.trim());
          cur = '';
        } else {
          cur += c;
        }
      }
      result.push(cur.trim());
      return result;
    };

    const headers = parseLine(lines[0]);
    const rows = lines.slice(1).map(parseLine);
    return { headers, rows };
  }, [previewType, textContent]);

  // Filtered CSV rows
  const filteredCSVRows = useMemo(() => {
    if (!searchQuery) return parsedCSV.rows;
    const q = searchQuery.toLowerCase();
    return parsedCSV.rows.filter((row) =>
      row.some((cell) => cell.toLowerCase().includes(q))
    );
  }, [parsedCSV.rows, searchQuery]);

  // Highlighted code lines
  const codeLines = useMemo(() => {
    return textContent.split('\n');
  }, [textContent]);

  const filteredCodeLines = useMemo(() => {
    if (!searchQuery) return codeLines;
    return codeLines.map((line) => {
      if (!line.toLowerCase().includes(searchQuery.toLowerCase())) return line;
      return line;
    });
  }, [codeLines, searchQuery]);

  // Basic syntax highlighter helper
  const renderHighlightedLine = (line: string) => {
    if (!line) return <span>&nbsp;</span>;

    // Highlighting tokens
    const parts: React.ReactNode[] = [];
    const tokenRegex = /(\/\/.*|#.*|\/\*[\s\S]*?\*\/)|("(?:\\.|[^"\\])*"|'[^']*'|`[^`]*`)|(\b(?:def|func|function|class|return|if|else|elif|import|from|export|default|const|let|var|type|interface|struct|package|for|range|while|switch|case|break|continue|try|except|catch|finally|async|await|select|insert|update|delete|where)\b)|(\b(?:true|false|null|nil|None|True|False|undefined)\b)|(\b\d+\b)/g;

    let lastIndex = 0;
    let match: RegExpExecArray | null;

    while ((match = tokenRegex.exec(line)) !== null) {
      if (match.index > lastIndex) {
        parts.push(line.substring(lastIndex, match.index));
      }
      if (match[1]) {
        parts.push(<span key={match.index} className="syn-comment">{match[1]}</span>);
      } else if (match[2]) {
        parts.push(<span key={match.index} className="syn-string">{match[2]}</span>);
      } else if (match[3]) {
        parts.push(<span key={match.index} className="syn-keyword">{match[3]}</span>);
      } else if (match[4]) {
        parts.push(<span key={match.index} className="syn-boolean">{match[4]}</span>);
      } else if (match[5]) {
        parts.push(<span key={match.index} className="syn-number">{match[5]}</span>);
      }
      lastIndex = tokenRegex.lastIndex;
    }

    if (lastIndex < line.length) {
      parts.push(line.substring(lastIndex));
    }

    return parts;
  };

  const getTypeLabel = () => {
    switch (previewType) {
      case 'markdown': return 'Markdown Canvas';
      case 'code': return `${ext.toUpperCase()} Source`;
      case 'csv': return 'Tabular Dataset';
      case 'presentation': return 'Presentation Deck';
      case 'office': return 'Office Document';
      case 'video': return 'Video Stream';
      case 'audio': return 'Audio Stream';
      case 'image': return 'High-Res Image';
      case 'pdf': return 'PDF Document';
      case 'text': return 'Plain Text';
      default: return 'Encrypted Binary';
    }
  };

  const getTypeIcon = () => {
    switch (previewType) {
      case 'markdown': return <BookOpen size={18} className="type-icon-markdown" />;
      case 'code': return <FileCode size={18} className="type-icon-code" />;
      case 'csv': return <FileSpreadsheet size={18} className="type-icon-sheet" />;
      case 'presentation': return <Presentation size={18} className="type-icon-ppt" />;
      case 'office': return <FileText size={18} className="type-icon-doc" />;
      case 'video': return <Film size={18} className="type-icon-video" />;
      case 'audio': return <Music size={18} className="type-icon-audio" />;
      case 'image': return <ImageIcon size={18} className="type-icon-image" />;
      case 'pdf': return <FileText size={18} className="type-icon-pdf" />;
      default: return <Layers size={18} className="type-icon-generic" />;
    }
  };

  // Video playback speed handler
  const handlePlaybackSpeed = (speed: number) => {
    setVideoPlaybackRate(speed);
    if (videoRef.current) {
      videoRef.current.playbackRate = speed;
    }
  };

  // Audio time format
  const formatTime = (seconds: number) => {
    if (isNaN(seconds) || seconds === 0) return '00:00';
    const mins = Math.floor(seconds / 60);
    const secs = Math.floor(seconds % 60);
    return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
  };

  // Audio play/pause
  const togglePlayAudio = () => {
    if (!audioRef.current) return;
    if (isPlayingAudio) {
      audioRef.current.pause();
      setIsPlayingAudio(false);
    } else {
      audioRef.current.play();
      setIsPlayingAudio(true);
    }
  };

  const handleAudioSeek = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = parseFloat(e.target.value);
    setAudioProgress(val);
    if (audioRef.current) {
      audioRef.current.currentTime = val;
    }
  };

  const renderContent = () => {
    switch (previewType) {
      case 'image':
        return (
          <div className="preview-image-viewport">
            <div
              className="preview-img-wrapper"
              style={{
                transform: `scale(${imgZoom})`,
                transition: 'transform 0.15s ease-out',
              }}
            >
              <img
                src={getPreviewURL(file.id)}
                alt={file.name}
                className="preview-img-element"
                onLoad={(e) => {
                  const target = e.currentTarget;
                  setImgNaturalDim({ w: target.naturalWidth, h: target.naturalHeight });
                }}
              />
            </div>
            <div className="image-zoom-toolbar">
              <button
                className="zoom-tool-btn"
                onClick={() => setImgZoom((z) => Math.max(0.2, z - 0.25))}
                title="Zoom Out"
              >
                <ZoomOut size={16} />
              </button>
              <span className="zoom-level-text">{Math.round(imgZoom * 100)}%</span>
              <button
                className="zoom-tool-btn"
                onClick={() => setImgZoom((z) => Math.min(4, z + 0.25))}
                title="Zoom In"
              >
                <ZoomIn size={16} />
              </button>
              <button
                className="zoom-tool-btn"
                onClick={() => setImgZoom(1)}
                title="Reset Zoom"
              >
                <RotateCcw size={15} />
              </button>
              {imgNaturalDim && (
                <span className="image-dim-badge">
                  {imgNaturalDim.w} × {imgNaturalDim.h} px
                </span>
              )}
            </div>
          </div>
        );

      case 'video':
        return (
          <div className="preview-video-viewport">
            <div className="video-player-container">
              <video
                ref={videoRef}
                controls
                autoPlay
                playsInline
                className="preview-video-element"
              >
                <source src={getStreamURL(file.id)} type={file.mime_type || 'video/mp4'} />
                Your browser does not support HTML5 video streaming.
              </video>
              <div className="video-speed-bar">
                <span className="speed-label">Speed:</span>
                {[0.75, 1, 1.25, 1.5, 2].map((s) => (
                  <button
                    key={s}
                    className={`speed-pill ${videoPlaybackRate === s ? 'active' : ''}`}
                    onClick={() => handlePlaybackSpeed(s)}
                  >
                    {s}x
                  </button>
                ))}
              </div>
            </div>
          </div>
        );

      case 'audio':
        return (
          <div className="preview-audio-viewport">
            <audio
              ref={audioRef}
              src={getStreamURL(file.id)}
              onTimeUpdate={() => {
                if (audioRef.current) {
                  setAudioProgress(audioRef.current.currentTime);
                  setAudioDuration(audioRef.current.duration || 0);
                }
              }}
              onLoadedMetadata={() => {
                if (audioRef.current) {
                  setAudioDuration(audioRef.current.duration || 0);
                }
              }}
              onEnded={() => setIsPlayingAudio(false)}
            />

            <div className="audio-card-studio">
              {/* Animated Waveform Equalizer */}
              <div className="audio-visual-wave">
                {Array.from({ length: 28 }).map((_, i) => (
                  <div
                    key={i}
                    className={`wave-bar ${isPlayingAudio ? 'animating' : ''}`}
                    style={{
                      height: isPlayingAudio
                        ? `${Math.max(15, (Math.sin(i * 0.4 + audioProgress * 3) * 0.5 + 0.5) * 60)}px`
                        : `${15 + (i % 5) * 4}px`,
                      animationDelay: `${i * 0.05}s`,
                    }}
                  ></div>
                ))}
              </div>

              <div className="audio-track-info">
                <h4 className="audio-track-title">{file.name}</h4>
                <p className="audio-track-meta">
                  {formatFileSize(file.size)} • AES-256 Decrypted Telegram Stream
                </p>
              </div>

              {/* Scrubber */}
              <div className="audio-scrub-row">
                <span className="audio-time-label">{formatTime(audioProgress)}</span>
                <input
                  type="range"
                  min={0}
                  max={audioDuration || 100}
                  step={0.1}
                  value={audioProgress}
                  onChange={handleAudioSeek}
                  className="audio-seek-slider"
                />
                <span className="audio-time-label">{formatTime(audioDuration)}</span>
              </div>

              {/* Controls */}
              <div className="audio-controls-row">
                <button className="audio-play-btn" onClick={togglePlayAudio}>
                  {isPlayingAudio ? <Pause size={24} /> : <Play size={24} className="ml-0-5" />}
                </button>

                <div className="audio-volume-group">
                  <button
                    className="volume-btn"
                    onClick={() => {
                      if (audioRef.current) {
                        const newMute = !isAudioMuted;
                        setIsAudioMuted(newMute);
                        audioRef.current.muted = newMute;
                      }
                    }}
                  >
                    {isAudioMuted || audioVolume === 0 ? <VolumeX size={18} /> : <Volume2 size={18} />}
                  </button>
                  <input
                    type="range"
                    min={0}
                    max={1}
                    step={0.05}
                    value={isAudioMuted ? 0 : audioVolume}
                    onChange={(e) => {
                      const v = parseFloat(e.target.value);
                      setAudioVolume(v);
                      setIsAudioMuted(false);
                      if (audioRef.current) {
                        audioRef.current.volume = v;
                        audioRef.current.muted = false;
                      }
                    }}
                    className="volume-slider"
                  />
                </div>
              </div>
            </div>
          </div>
        );

      case 'markdown':
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
                <span>{textContent.split(/\s+/).filter(Boolean).length} words</span>
                <span>•</span>
                <span>{codeLines.length} lines</span>
              </div>
            </div>

            {loadingText ? (
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

      case 'code':
      case 'text':
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

            {loadingText ? (
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

      case 'csv':
        return (
          <div className="preview-csv-viewport">
            <div className="canvas-sub-header">
              <div className="canvas-mode-toggle">
                <button
                  className={`mode-tab-btn ${csvMode === 'table' ? 'active' : ''}`}
                  onClick={() => setCsvMode('table')}
                >
                  <Table size={14} />
                  <span>Table View</span>
                </button>
                <button
                  className={`mode-tab-btn ${csvMode === 'raw' ? 'active' : ''}`}
                  onClick={() => setCsvMode('raw')}
                >
                  <Code2 size={14} />
                  <span>Raw Text</span>
                </button>
              </div>

              <div className="search-in-code-box">
                <Search size={14} className="search-icon" />
                <input
                  type="text"
                  placeholder="Filter rows..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="search-input-field"
                />
              </div>

              <div className="canvas-stats">
                <span>{parsedCSV.headers.length} columns</span>
                <span>•</span>
                <span>{filteredCSVRows.length} rows</span>
              </div>
            </div>

            {loadingText ? (
              <div className="preview-loading-state">
                <span className="spinner-border"></span>
                <p>Decrypting dataset...</p>
              </div>
            ) : csvMode === 'table' ? (
              <div className="csv-table-scroll-container">
                <table className="csv-interactive-table">
                  <thead>
                    <tr>
                      <th className="table-index-col">#</th>
                      {parsedCSV.headers.map((h, i) => (
                        <th key={i}>{h}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {filteredCSVRows.map((row, rIdx) => (
                      <tr key={rIdx}>
                        <td className="table-index-col">{rIdx + 1}</td>
                        {row.map((cell, cIdx) => (
                          <td key={cIdx}>{cell}</td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <div className="raw-code-canvas">
                <pre className="code-pre-block">
                  <code>{textContent}</code>
                </pre>
              </div>
            )}
          </div>
        );

      case 'presentation':
        return (
          <div className="preview-presentation-viewport">
            <div className="deck-studio-card">
              <div className="deck-screen-frame">
                <div className="deck-slide-mockup">
                  <div className="deck-slide-header-bar">
                    <span className="deck-badge-pill">PowerPoint Presentation</span>
                    <span className="deck-slide-number">Slide Deck</span>
                  </div>
                  <div className="deck-slide-body">
                    <Presentation size={64} className="deck-big-icon" />
                    <h3 className="deck-file-title">{file.name}</h3>
                    <p className="deck-file-desc">
                      Encrypted in {file.chunk_count} AES-256 segment(s) • {formatFileSize(file.size)}
                    </p>
                  </div>
                </div>
              </div>

              <div className="deck-actions-panel">
                <div className="deck-info-box">
                  <ShieldCheck size={20} className="text-emerald" />
                  <div>
                    <h5 className="deck-info-title">Hardware Encrypted Slide Deck</h5>
                    <p className="deck-info-sub">
                      All slides, speaker notes, and embedded media are securely stored in chunks on Telegram.
                    </p>
                  </div>
                </div>

                <div className="deck-buttons-row">
                  <button className="btn-primary-glow btn-deck-download" onClick={handleDownload}>
                    <Download size={18} />
                    <span>Download & Decrypt Deck</span>
                  </button>
                  <a
                    href={getStreamURL(file.id)}
                    target="_blank"
                    rel="noreferrer"
                    className="btn-secondary-glass"
                  >
                    <ExternalLink size={16} />
                    <span>Direct Stream</span>
                  </a>
                </div>
              </div>
            </div>
          </div>
        );

      case 'office':
        return (
          <div className="preview-presentation-viewport">
            <div className="deck-studio-card office-card">
              <div className="deck-screen-frame">
                <div className="deck-slide-mockup">
                  <div className="deck-slide-header-bar">
                    <span className="deck-badge-pill">Office Document</span>
                    <span className="deck-slide-number">{ext.toUpperCase()}</span>
                  </div>
                  <div className="deck-slide-body">
                    <FileText size={64} className="deck-big-icon text-cyan" />
                    <h3 className="deck-file-title">{file.name}</h3>
                    <p className="deck-file-desc">
                      {formatFileSize(file.size)} • {file.chunk_count} encrypted chunk(s)
                    </p>
                  </div>
                </div>
              </div>

              <div className="deck-actions-panel">
                <div className="deck-info-box">
                  <ShieldCheck size={20} className="text-emerald" />
                  <div>
                    <h5 className="deck-info-title">Secure Office Document</h5>
                    <p className="deck-info-sub">
                      Decrypt and edit in Microsoft 365, LibreOffice, or Google Docs.
                    </p>
                  </div>
                </div>

                <div className="deck-buttons-row">
                  <button className="btn-primary-glow btn-deck-download" onClick={handleDownload}>
                    <Download size={18} />
                    <span>Download Decrypted Document</span>
                  </button>
                </div>
              </div>
            </div>
          </div>
        );

      case 'pdf':
        return (
          <div className="preview-pdf-viewport">
            <iframe
              src={getPreviewURL(file.id)}
              className="preview-pdf-frame"
              title={file.name}
            />
          </div>
        );

      default:
        return (
          <div className="preview-unsupported-viewport">
            <div className="unsupported-card">
              <Layers size={54} className="unsupported-icon" />
              <h4 className="unsupported-title">{file.name}</h4>
              <p className="unsupported-sub">
                Binary format • {formatFileSize(file.size)} • {file.chunk_count} chunk(s)
              </p>
              <div className="unsupported-telemetry">
                <div className="tele-item">
                  <span className="tele-lbl">MIME</span>
                  <span className="tele-val">{file.mime_type || 'application/octet-stream'}</span>
                </div>
                <div className="tele-item">
                  <span className="tele-lbl">CIPHER</span>
                  <span className="tele-val">AES-256-GCM</span>
                </div>
                <div className="tele-item">
                  <span className="tele-lbl">SALT</span>
                  <span className="tele-val font-mono">{file.encryption_salt.substring(0, 16)}...</span>
                </div>
              </div>
              <button className="btn-primary-glow" onClick={handleDownload}>
                <Download size={16} />
                <span>Download & Decrypt File</span>
              </button>
            </div>
          </div>
        );
    }
  };

  return (
    <div className="preview-backdrop" onClick={onClose}>
      <div className="preview-window-glass" onClick={(e) => e.stopPropagation()}>
        {/* ChatGPT Style Top Header */}
        <div className="preview-window-header">
          <div className="preview-title-group">
            <div className="type-icon-badge">{getTypeIcon()}</div>
            <div className="title-text-wrap">
              <div className="title-flex-row">
                <h3 className="window-filename" title={file.name}>{file.name}</h3>
                <span className="canvas-type-tag">{getTypeLabel()}</span>
              </div>
              <div className="title-meta-pills">
                <span className="meta-pill">{formatFileSize(file.size)}</span>
                <span className="meta-pill">{file.chunk_count} {file.chunk_count === 1 ? 'chunk' : 'chunks'}</span>
                <span className="meta-pill cipher-pill">AES-256-GCM</span>
              </div>
            </div>
          </div>

          <div className="preview-window-controls">
            {textContent && (
              <button className="icon-action-btn" onClick={handleCopy} title="Copy Content">
                {copied ? <Check size={17} className="text-emerald" /> : <Copy size={17} />}
              </button>
            )}
            <button className="icon-action-btn" onClick={handleDownload} title="Download Decrypted">
              <Download size={17} />
            </button>
            <a
              href={getStreamURL(file.id)}
              target="_blank"
              rel="noreferrer"
              className="icon-action-btn"
              title="Open stream in new tab"
            >
              <ExternalLink size={17} />
            </a>
            <button className="icon-action-btn close-btn" onClick={onClose} title="Close Preview (Esc)">
              <X size={19} />
            </button>
          </div>
        </div>

        {/* Viewport Content */}
        <div className="preview-window-body">
          {renderContent()}
        </div>
      </div>
    </div>
  );
}
