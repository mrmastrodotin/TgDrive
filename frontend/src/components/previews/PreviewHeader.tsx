import React from 'react';
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
  PictureInPicture2,
  Maximize2,
} from 'lucide-react';
import { FileItem, PreviewableType, formatFileSize } from '../../types';
import { getStreamURL } from '../../api/client';

export interface PreviewHeaderProps {
  file: FileItem;
  previewType: PreviewableType;
  isPiPMode: boolean;
  isPiPActive: boolean;
  textContent?: string;
  copied?: boolean;
  onCopy?: () => void;
  onDownload: () => void;
  onTogglePiP: () => void;
  onClose: () => void;
  onDockToBottomPlayer?: (file: FileItem) => void;
}

export const PreviewHeader: React.FC<PreviewHeaderProps> = ({
  file,
  previewType,
  isPiPMode,
  isPiPActive,
  textContent,
  copied,
  onCopy,
  onDownload,
  onTogglePiP,
  onClose,
  onDockToBottomPlayer,
}) => {
  const getTypeIcon = () => {
    switch (previewType) {
      case 'video':
        return <Film size={18} className="text-primary" />;
      case 'audio':
        return <Music size={18} className="text-primary" />;
      case 'image':
        return <ImageIcon size={18} className="text-secondary" />;
      case 'markdown':
        return <BookOpen size={18} className="text-primary" />;
      case 'code':
        return <FileCode size={18} className="text-primary" />;
      case 'csv':
        return <FileSpreadsheet size={18} className="text-secondary" />;
      case 'presentation':
        return <Presentation size={18} className="text-primary" />;
      case 'office':
        return <FileText size={18} className="text-primary" />;
      case 'pdf':
        return <FileText size={18} className="text-secondary" />;
      default:
        return <FileText size={18} className="text-outline" />;
    }
  };

  const getTypeLabel = () => {
    switch (previewType) {
      case 'video':
        return 'Cinematic Video';
      case 'audio':
        return 'Lossless Audio';
      case 'image':
        return 'Image Canvas';
      case 'markdown':
        return 'Markdown Doc';
      case 'code':
        return 'Source Code';
      case 'csv':
        return 'Dataset Table';
      case 'presentation':
        return 'Slide Deck';
      case 'office':
        return 'Office Doc';
      case 'pdf':
        return 'PDF Document';
      default:
        return 'Encrypted Binary';
    }
  };

  return (
    <div className="preview-window-header">
      <div className="preview-title-group">
        <div className="type-icon-badge">{getTypeIcon()}</div>
        <div className="title-text-wrap">
          <div className="title-flex-row">
            <h3 className="window-filename" title={file.name}>
              {file.name}
            </h3>
            {!isPiPActive && (
              <span className="canvas-type-tag">
                {getTypeLabel()}
              </span>
            )}
          </div>
          {!isPiPActive && (
            <div className="title-meta-pills">
              <span className="meta-pill">{formatFileSize(file.size)}</span>
              <span className="meta-pill">
                {file.chunk_count} {file.chunk_count === 1 ? 'chunk' : 'chunks'}
              </span>
              <span className="meta-pill cipher-pill">AES-256-GCM</span>
            </div>
          )}
        </div>
      </div>

      <div className="preview-window-controls">
        {/* Single Picture-in-Picture / Restore Button */}
        <button
          className={`icon-action-btn pip-btn ${isPiPActive ? 'active' : ''}`}
          onClick={onTogglePiP}
          title={isPiPActive ? 'Expand to Full Window (P)' : 'Picture-in-Picture (P)'}
        >
          {isPiPActive ? <Maximize2 size={17} /> : <PictureInPicture2 size={17} />}
        </button>

        {!isPiPActive && textContent && onCopy && (
          <button className="icon-action-btn" onClick={onCopy} title="Copy Content">
            {copied ? <Check size={17} className="text-primary" /> : <Copy size={17} />}
          </button>
        )}

        {!isPiPActive && (
          <button className="icon-action-btn" onClick={onDownload} title="Download Decrypted">
            <Download size={17} />
          </button>
        )}

        {!isPiPActive && (
          <a
            href={getStreamURL(file.id)}
            target="_blank"
            rel="noreferrer"
            className="icon-action-btn"
            title="Open stream in new tab"
          >
            <ExternalLink size={17} />
          </a>
        )}

        <button className="icon-action-btn close-btn" onClick={onClose} title="Close Preview (Esc)">
          <X size={19} />
        </button>
      </div>
    </div>
  );
};
