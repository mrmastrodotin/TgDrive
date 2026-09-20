import React, { useState, useEffect } from 'react';
import { FileItem, getPreviewType } from '../types';
import { getStreamURL, getPreviewURL } from '../api/client';

interface FilePreviewProps {
  file: FileItem;
  onClose: () => void;
}

export default function FilePreview({ file, onClose }: FilePreviewProps) {
  const previewType = getPreviewType(file.mime_type, file.name);
  const token = localStorage.getItem('tgdrive_token') || '';
  const [textContent, setTextContent] = useState<string>('');

  // Build authenticated URL
  const authURL = (url: string) => `${url}?token=${token}`;

  useEffect(() => {
    if (previewType === 'text' || previewType === 'code') {
      fetch(getPreviewURL(file.id), {
        headers: { 'Authorization': `Bearer ${token}` },
      })
        .then((res) => res.text())
        .then(setTextContent)
        .catch(() => setTextContent('Failed to load file'));
    }
  }, [file.id, previewType, token]);

  const renderPreview = () => {
    switch (previewType) {
      case 'image':
        return (
          <div className="preview-image-container">
            <img
              src={getPreviewURL(file.id)}
              alt={file.name}
              className="preview-image"
              crossOrigin="anonymous"
            />
          </div>
        );

      case 'video':
        return (
          <div className="preview-video-container">
            <video
              controls
              autoPlay
              className="preview-video"
              crossOrigin="anonymous"
            >
              <source src={getStreamURL(file.id)} type={file.mime_type} />
            </video>
          </div>
        );

      case 'audio':
        return (
          <div className="preview-audio-container">
            <div className="preview-audio-icon">🎵</div>
            <p className="preview-audio-name">{file.name}</p>
            <audio controls autoPlay className="preview-audio" crossOrigin="anonymous">
              <source src={getStreamURL(file.id)} type={file.mime_type} />
            </audio>
          </div>
        );

      case 'pdf':
        return (
          <div className="preview-pdf-container">
            <iframe
              src={getPreviewURL(file.id)}
              className="preview-pdf"
              title={file.name}
            />
          </div>
        );

      case 'text':
      case 'code':
        return (
          <div className="preview-text-container">
            <pre className="preview-text">{textContent}</pre>
          </div>
        );

      default:
        return (
          <div className="preview-unsupported">
            <p>Preview not available for this file type</p>
          </div>
        );
    }
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="preview-modal" onClick={(e) => e.stopPropagation()}>
        <div className="preview-header">
          <h3 className="preview-title">{file.name}</h3>
          <button id="preview-close" className="modal-close" onClick={onClose}>✕</button>
        </div>
        <div className="preview-body">
          {renderPreview()}
        </div>
      </div>
    </div>
  );
}
