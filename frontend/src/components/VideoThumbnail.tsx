import React, { useState, useEffect, useRef } from 'react';
import { FileItem } from '../types';
import { getScopedStreamURL, getToken } from '../api/client';
import { Film } from 'lucide-react';

interface VideoThumbnailProps {
  file: FileItem;
  className?: string;
  isHero?: boolean;
}

export const VideoThumbnail: React.FC<VideoThumbnailProps> = ({
  file,
  className = '',
  isHero = false,
}) => {
  const [hasError, setHasError] = useState(false);
  const [firstFrameReady, setFirstFrameReady] = useState(false);
  const [capturedFrameUrl, setCapturedFrameUrl] = useState<string | null>(null);
  const [streamUrl, setStreamUrl] = useState<string>('');
  const videoRef = useRef<HTMLVideoElement>(null);

  // 1. Fetch thumbnail from metadata if available
  const metadataThumb = file.metadata?.thumbnail_telegram_file_id 
    ? `/api/files/${file.id}/thumbnail?token=${getToken()}`
    : (file as any).thumbnail_url || (file as any).thumbnail || (file as any).poster;

  useEffect(() => {
    if (metadataThumb) return;
    let isCancelled = false;
    getScopedStreamURL(file.id)
      .then((url) => {
        if (!isCancelled) setStreamUrl(url);
      })
      .catch(() => {});
    return () => {
      isCancelled = true;
    };
  }, [file.id, metadataThumb]);

  // 2. Fall back to extracting first frame from the video stream
  useEffect(() => {
    if (metadataThumb || !streamUrl) return;

    setHasError(false);
    setFirstFrameReady(false);
    setCapturedFrameUrl(null);

    const video = document.createElement('video');
    video.muted = true;
    video.playsInline = true;
    video.preload = 'metadata';
    // Append #t=0.5 to encourage browser media engine to decode the first keyframe
    video.src = `${streamUrl}#t=0.5`;

    let isCancelled = false;

    const captureFrame = () => {
      if (isCancelled) return;
      try {
        if (video.videoWidth > 0 && video.videoHeight > 0) {
          const canvas = document.createElement('canvas');
          canvas.width = Math.min(video.videoWidth, isHero ? 1920 : 640);
          canvas.height = Math.min(video.videoHeight, isHero ? 1080 : 360);
          const ctx = canvas.getContext('2d');
          if (ctx) {
            ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
            const dataUrl = canvas.toDataURL('image/jpeg', 0.85);
            setCapturedFrameUrl(dataUrl);
            setFirstFrameReady(true);
            return;
          }
        }
        setFirstFrameReady(true);
      } catch {
        // Cross-origin restriction or tainted canvas; fallback to direct video element
        setFirstFrameReady(true);
      }
    };

    const handleLoadedData = () => {
      captureFrame();
    };

    const handleError = () => {
      if (!isCancelled) {
        setHasError(true);
      }
    };

    video.addEventListener('loadeddata', handleLoadedData);
    video.addEventListener('seeked', handleLoadedData);
    video.addEventListener('error', handleError);

    // Timeout safety for offline mock files or unsupported container codecs
    const timer = setTimeout(() => {
      if (!isCancelled && !firstFrameReady) {
        setFirstFrameReady(true);
      }
    }, 4000);

    return () => {
      isCancelled = true;
      clearTimeout(timer);
      video.removeEventListener('loadeddata', handleLoadedData);
      video.removeEventListener('seeked', handleLoadedData);
      video.removeEventListener('error', handleError);
      video.src = '';
    };
  }, [file.id, metadataThumb, isHero]);

  // Priority 1: Metadata thumbnail
  if (metadataThumb && !hasError) {
    return (
      <img
        src={metadataThumb}
        alt={file.name}
        className={`w-full h-full object-cover object-center ${className}`}
        onError={() => setHasError(true)}
      />
    );
  }

  // Priority 2: Canvas-captured first frame from video stream
  if (capturedFrameUrl) {
    return (
      <img
        src={capturedFrameUrl}
        alt={file.name}
        className={`w-full h-full object-cover object-center ${className}`}
      />
    );
  }

  // Priority 3: Direct video element displaying decoded first frame
  if (!hasError) {
    return (
      <div className={`relative w-full h-full overflow-hidden bg-surface-container-lowest ${className}`}>
        <video
          ref={videoRef}
          src={streamUrl ? `${streamUrl}#t=0.5` : undefined}
          preload="metadata"
          muted
          playsInline
          className="w-full h-full object-cover object-center pointer-events-none"
          onLoadedData={() => setFirstFrameReady(true)}
          onError={() => setHasError(true)}
        />
        {!firstFrameReady && (
          <div className="absolute inset-0 bg-surface-container-low/70 backdrop-blur-xs flex items-center justify-center animate-pulse">
            <Film size={isHero ? 48 : 28} className="text-primary/40" />
          </div>
        )}
      </div>
    );
  }

  // Fallback: Elegant cinematic Obsidian Gold poster frame when video stream/codec cannot be decoded in browser
  return (
    <div
      className={`w-full h-full flex flex-col items-center justify-center bg-gradient-to-br from-[#241f18] via-[#161514] to-[#0c0c0b] text-primary/80 border border-outline-variant/15 p-4 text-center ${className}`}
    >
      <div className={`${isHero ? 'w-20 h-20' : 'w-12 h-12'} rounded-2xl bg-primary/10 border border-primary/25 flex items-center justify-center mb-2 shadow-[0_0_25px_rgba(252,186,105,0.18)]`}>
        <Film size={isHero ? 38 : 24} className="text-primary" />
      </div>
      <span className="text-xs font-mono uppercase tracking-widest text-on-surface-variant font-bold">
        {file.name.split('.').pop()?.toUpperCase() || 'VIDEO'} • AES-256
      </span>
      <span className="text-[11px] text-on-surface-variant/60 line-clamp-1 max-w-[200px] mt-0.5 font-medium">
        {file.name}
      </span>
    </div>
  );
};
