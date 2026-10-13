import React from 'react';
import { FileItem } from '../../types';
import { getPreviewURL } from '../../api/client';

export interface PdfPreviewProps {
  file: FileItem;
}

export const PdfPreview: React.FC<PdfPreviewProps> = ({ file }) => {
  return (
    <div className="preview-pdf-viewport">
      <iframe
        src={getPreviewURL(file.id)}
        className="preview-pdf-frame"
        title={file.name}
      />
    </div>
  );
};
