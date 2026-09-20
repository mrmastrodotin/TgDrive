import React, { useState, useEffect } from 'react';
import {
  FileItem,
  getPreviewType,
  formatFileSize,
} from '../types';
import { getStreamURL, getPreviewURL, downloadFile } from '../api/client';
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
} from 'lucide-react';

interface FilePreviewProps {
  file: FileItem;
  onClose: () => void;
}

export default function FilePreview({ file, onClose }: FilePreviewProps) {
  const previewType = getPreviewType(file.mime_type, file.name);
  const [textContent, setTextContent] = useState<string>('');
  const [loadingText, setLoadingText] = useState(false);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (previewType === 'text' || previewType === 'code') {
      setLoadingText(true);
      fetch(getPreviewURL(file.id))
        .then((res) => {
          if (!res.ok) throw new Error('Failed to load text preview');
          return res.text();
        })
        .then((text) => {
          setTextContent(text);
          setLoadingText(false);
        })
        .catch((err) => {
          setTextContent(`// Error loading preview: ${err.message}`);
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

  const renderContent = () => {
    switch (previewType) {
      case 'image':
        return (
          <div className="preview-image-viewport">
            <img
              src={getPreviewURL(file.id)}
              alt={file.name}
              className="preview-img-element"
            />
          </div>
        );

      case 'video':
        return (
          <div className="preview-video-viewport">
            <video
              controls
              autoPlay
              playsInline
              className="preview-video-element"
            >
              <source src={getStreamURL(file.id)} type={file.mime_type || 'video/mp4'} />
              Your browser does not support HTML5 video streaming.
            </video>
          </div>
        );

      case 'audio':
        return (
          <div className="preview-audio-viewport">
            <div className="audio-visual-pulse">
              <Music size={48} className="audio-icon-pulse" />
            </div>
            <h4 className="audio-track-title">{file.name}</h4>
            <p className="audio-track-meta">{formatFileSize(file.size)} • Encrypted Stream</p>
            <audio controls autoPlay className="preview-audio-element">
              <source src={getStreamURL(file.id)} type={file.mime_type || 'audio/mpeg'} />
              Your browser does not support HTML5 audio.
            </audio>
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

      case 'text':
      case 'code':
        return (
          <div className="preview-code-viewport">
            <div className="code-header-bar">
              <span className="code-lang-tag">{file.name.split('.').pop()?.toUpperCase()}</span>
              <button className="copy-code-btn" onClick={handleCopy}>
                {copied ? (
                  <>
                    <Check size={14} className="text-emerald" />
                    <span>Copied!</span>
                  </>
                ) : (
                  <>
                    <Copy size={14} />
                    <span>Copy All</span>
                  </>
                )}
              </button>
            </div>
            {loadingText ? (
              <div className="preview-loading-state">
                <span className="spinner-border"></span>
                <p>Decrypting content from Telegram...</p>
              </div>
            ) : (
              <pre className="code-pre-block">
                <code>{textContent}</code>
              </pre>
            )}
          </div>
        );

      default:
        return (
          <div className="preview-unsupported-viewport">
            <p>Direct preview is unavailable for this binary format.</p>
            <button className="btn-primary-glow" onClick={handleDownload}>
              <Download size={16} />
              <span>Download & Decrypt</span>
            </button>
          </div>
        );
    }
  };

  const getTypeIcon = () => {
    switch (previewType) {
      case 'video': return <Film size={18} className="type-icon-video" />;
      case 'image': return <ImageIcon size={18} className="type-icon-image" />;
      case 'audio': return <Music size={18} className="type-icon-audio" />;
      case 'code': return <FileCode size={18} className="type-icon-code" />;
      default: return <FileText size={18} className="type-icon-doc" />;
    }
  };

  return (
    <div className="preview-backdrop" onClick={onClose}>
      <div className="preview-window-glass" onClick={(e) => e.stopPropagation()}>
        <div className="preview-window-header">
          <div className="preview-title-group">
            <div className="type-icon-badge">{getTypeIcon()}</div>
            <div className="title-text-wrap">
              <h3 className="window-filename" title={file.name}>{file.name}</h3>
              <span className="window-filesize">{formatFileSize(file.size)} • {file.chunk_count} chunk(s)</span>
            </div>
          </div>

          <div className="preview-window-controls">
            <button className="icon-action-btn" onClick={handleDownload} title="Download Decrypted">
              <Download size={18} />
            </button>
            <a
              href={getStreamURL(file.id)}
              target="_blank"
              rel="noreferrer"
              className="icon-action-btn"
              title="Open stream in new tab"
            >
              <ExternalLink size={18} />
            </a>
            <button className="icon-action-btn close-btn" onClick={onClose} title="Close Preview (Esc)">
              <X size={20} />
            </button>
          </div>
        </div>

        <div className="preview-window-body">
          {renderContent()}
        </div>
      </div>
    </div>
  );
}
