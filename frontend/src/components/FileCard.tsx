import React from 'react';
import {
  FileItem,
  formatFileSize,
  getPreviewType,
  getFileCategory,
} from '../types';
import { downloadFile } from '../api/client';
import {
  Film,
  Image as ImageIcon,
  Music,
  FileText,
  FileCode,
  FileArchive,
  File as FileGeneric,
  Eye,
  Download,
  Trash2,
  Lock,
} from 'lucide-react';

interface FileCardProps {
  file: FileItem;
  viewMode?: 'grid' | 'list';
  onDelete: (id: string) => void;
  onPreview: (file: FileItem) => void;
}

export default function FileCard({ file, viewMode = 'grid', onDelete, onPreview }: FileCardProps) {
  const previewType = getPreviewType(file.mime_type, file.name);
  const category = getFileCategory(file.mime_type, file.name);
  const date = new Date(file.created_at).toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  });

  const getCategoryDetails = () => {
    switch (category) {
      case 'media':
        if (file.mime_type.startsWith('video/')) {
          return { icon: <Film size={viewMode === 'grid' ? 28 : 20} />, colorClass: 'cat-video' };
        }
        if (file.mime_type.startsWith('audio/')) {
          return { icon: <Music size={viewMode === 'grid' ? 28 : 20} />, colorClass: 'cat-audio' };
        }
        return { icon: <ImageIcon size={viewMode === 'grid' ? 28 : 20} />, colorClass: 'cat-image' };
      case 'code':
        return { icon: <FileCode size={viewMode === 'grid' ? 28 : 20} />, colorClass: 'cat-code' };
      case 'archives':
        return { icon: <FileArchive size={viewMode === 'grid' ? 28 : 20} />, colorClass: 'cat-archive' };
      case 'docs':
      default:
        if (file.mime_type === 'application/pdf') {
          return { icon: <FileText size={viewMode === 'grid' ? 28 : 20} />, colorClass: 'cat-pdf' };
        }
        return { icon: <FileGeneric size={viewMode === 'grid' ? 28 : 20} />, colorClass: 'cat-doc' };
    }
  };

  const { icon, colorClass } = getCategoryDetails();

  const handleDownload = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await downloadFile(file.id, file.name);
    } catch (err) {
      console.error('Download failed:', err);
    }
  };

  const handleDelete = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (confirm(`Delete "${file.name}"? This removes all chunks from Telegram.`)) {
      onDelete(file.id);
    }
  };

  if (viewMode === 'list') {
    return (
      <div
        className={`file-row-glass ${colorClass}`}
        onClick={() => previewType !== 'none' && onPreview(file)}
        tabIndex={0}
      >
        <div className="file-row-main">
          <div className="file-icon-badge">{icon}</div>
          <div className="file-name-meta">
            <span className="file-row-name" title={file.name}>{file.name}</span>
            <span className="file-row-ext">{file.name.split('.').pop()?.toUpperCase()}</span>
          </div>
        </div>

        <div className="file-row-details">
          <span className="badge-chunks">
            <Lock size={11} className="inline-lock" />
            {file.chunk_count} {file.chunk_count === 1 ? 'chunk' : 'chunks'}
          </span>
          <span className="file-row-size">{formatFileSize(file.size)}</span>
          <span className="file-row-date">{date}</span>
        </div>

        <div className="file-row-actions">
          {previewType !== 'none' && (
            <button
              className="action-btn action-preview"
              onClick={(e) => { e.stopPropagation(); onPreview(file); }}
              title="Stream / Preview"
            >
              <Eye size={15} />
            </button>
          )}
          <button
            className="action-btn action-download"
            onClick={handleDownload}
            title="Download Decrypted"
          >
            <Download size={15} />
          </button>
          <button
            className="action-btn action-delete"
            onClick={handleDelete}
            title="Delete File"
          >
            <Trash2 size={15} />
          </button>
        </div>
      </div>
    );
  }

  return (
    <div
      className={`file-card-glass ${colorClass}`}
      onClick={() => previewType !== 'none' && onPreview(file)}
      role="button"
      tabIndex={0}
    >
      <div className="file-card-top">
        <div className="file-icon-container">
          {icon}
        </div>
        <div className="file-badges-row">
          <span className="chunk-tag">
            <Lock size={10} className="tag-lock-icon" />
            {file.chunk_count} {file.chunk_count === 1 ? 'chunk' : 'chunks'}
          </span>
        </div>
      </div>

      <div className="file-card-body">
        <h4 className="file-title-text" title={file.name}>{file.name}</h4>
        <div className="file-meta-sub">
          <span className="file-size-tag">{formatFileSize(file.size)}</span>
          <span className="meta-separator">•</span>
          <span className="file-date-tag">{date}</span>
        </div>
      </div>

      <div className="file-card-hover-actions">
        {previewType !== 'none' && (
          <button
            className="card-action-btn action-preview"
            onClick={(e) => { e.stopPropagation(); onPreview(file); }}
            title="Live Stream / Preview"
          >
            <Eye size={15} />
            <span>View</span>
          </button>
        )}
        <button
          className="card-action-btn action-download"
          onClick={handleDownload}
          title="Download Decrypted"
        >
          <Download size={15} />
          <span>Save</span>
        </button>
        <button
          className="card-action-btn action-delete"
          onClick={handleDelete}
          title="Delete from Telegram"
        >
          <Trash2 size={15} />
        </button>
      </div>
    </div>
  );
}
