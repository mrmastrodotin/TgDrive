import React, { useState } from 'react';
import { ZoomIn, ZoomOut, RotateCcw } from 'lucide-react';
import { FileItem } from '../../types';
import { getPreviewURL } from '../../api/client';

export interface ImagePreviewProps {
  file: FileItem;
}

export const ImagePreview: React.FC<ImagePreviewProps> = ({ file }) => {
  const [imgZoom, setImgZoom] = useState(1);
  const [imgNaturalDim, setImgNaturalDim] = useState<{ w: number; h: number } | null>(null);
  const [imgSrc, setImgSrc] = useState<string>('');

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
          src={imgSrc || getPreviewURL(file.id)}
          alt={file.name}
          className="preview-img-element"
          onError={(e) => {
            // Optional: set to a generic error icon or just leave broken image.
            // For now, we do nothing and let it show broken image icon or hide it.
            // e.currentTarget.style.display = 'none';
          }}
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
};
