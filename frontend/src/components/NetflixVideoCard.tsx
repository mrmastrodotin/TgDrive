import React from 'react';
import { Play, Info, Plus, MoreVertical, Heart, ShieldCheck, Check } from 'lucide-react';
import { FileItem, formatFileSize } from '../types';
import { VideoThumbnail } from './VideoThumbnail';

interface NetflixVideoCardProps {
  file: FileItem;
  className?: string;
  onPlay: (file: FileItem) => void;
  onOpenInfo: (file: FileItem) => void;
  onAddToCategory?: (file: FileItem) => void;
  onToggleFavorite?: (fileId: string) => void;
  isFavorite?: boolean;
  onOpenMenu?: (e: React.MouseEvent, file: FileItem) => void;
  progressPercent?: number;
}

export function getCleanMovieTitle(filename: string): string {
  const withoutExt = filename.replace(/\.[^/.]+$/, '');
  return withoutExt.replace(/[._]/g, ' ').replace(/\s+/g, ' ').trim();
}

export const NetflixVideoCard: React.FC<NetflixVideoCardProps> = ({
  file,
  className = "min-w-[280px] sm:min-w-[310px] max-w-[340px]",
  onPlay,
  onOpenInfo,
  onAddToCategory,
  onToggleFavorite,
  isFavorite = false,
  onOpenMenu,
  progressPercent,
}) => {
  const cleanTitle = getCleanMovieTitle(file.name);
  const is4K = file.size > 8 * 1024 * 1024 * 1024;
  const is1080p = file.size > 1.5 * 1024 * 1024 * 1024 && !is4K;
  const resolutionTag = is4K ? '4K UHD' : is1080p ? '1080p' : 'HD';
  const durationStr = file.metadata?.duration ? `${Math.floor(file.metadata.duration / 60)}m` : null;

  const isMKV = file.name.toLowerCase().endsWith('.mkv') || file.mime_type.includes('matroska');
  const codecTag = isMKV ? 'HEVC' : 'H.264';

  const matchPercent = 94 + (Math.abs(file.name.split('').reduce((acc, c) => acc + c.charCodeAt(0), 0)) % 6);

  return (
    <div
      className={`group relative aspect-[16/10] flex-shrink-0 rounded-xl overflow-hidden bg-surface-container shadow-md transition-all duration-300 border border-outline-variant/20 hover:border-primary/30 ${className}`}
    >
      {/* Background Thumbnail */}
      <div className="absolute inset-0 w-full h-full">
        <VideoThumbnail file={file} className="group-hover:scale-105 transition-transform duration-700 ease-out" />
      </div>

      {/* Gradients */}
      <div className="absolute inset-0 bg-gradient-to-t from-[#141414] via-[#141414]/60 to-transparent opacity-80 group-hover:opacity-95 transition-opacity duration-300 pointer-events-none" />
      <div className="absolute inset-0 bg-gradient-to-b from-black/50 via-transparent to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300 pointer-events-none" />

      {/* Top Right Actions (Revealed on hover) */}
      <div className="absolute top-3 right-3 flex flex-col gap-2 z-20 opacity-0 group-hover:opacity-100 transition-all duration-300 translate-x-4 group-hover:translate-x-0">
        {onToggleFavorite && (
          <button
            onClick={(e) => { e.stopPropagation(); onToggleFavorite(file.id); }}
            className={`cursor-pointer w-8 h-8 rounded-full bg-black/50 hover:bg-black/80 backdrop-blur-md flex items-center justify-center border border-white/20 transition-all ${isFavorite ? 'text-[#E50914]' : 'text-white'}`}
            title={isFavorite ? 'Remove Favorite' : 'Add to Favorites'}
          >
            <Heart size={14} fill={isFavorite ? 'currentColor' : 'none'} />
          </button>
        )}
        {onOpenMenu && (
          <button
            onClick={(e) => { e.stopPropagation(); onOpenMenu(e, file); }}
            className="cursor-pointer w-8 h-8 rounded-full bg-black/50 hover:bg-black/80 backdrop-blur-md flex items-center justify-center border border-white/20 transition-all text-white"
          >
            <MoreVertical size={16} />
          </button>
        )}
      </div>

      {/* Bottom Content Area */}
      <div className="absolute bottom-0 left-0 right-0 p-4 flex flex-col justify-end z-20 pointer-events-none">
        
        {/* Title */}
        <h4 className="font-extrabold text-white text-lg leading-tight line-clamp-1 mb-1.5 drop-shadow-[0_2px_4px_rgba(0,0,0,0.8)] transform translate-y-4 group-hover:translate-y-0 transition-transform duration-500">
          {cleanTitle}
        </h4>

        {/* Meta Info Row */}
        <div className="flex items-center gap-2 text-xs font-semibold text-white/90 drop-shadow-md transform translate-y-4 group-hover:translate-y-0 transition-transform duration-500 delay-75">
          {durationStr && <span>{durationStr}</span>}
          <span className="px-1.5 py-0.5 rounded border border-white/30 text-[10px] text-white/80 bg-white/10 backdrop-blur-sm">
            {resolutionTag}
          </span>
          <span className="px-1.5 py-0.5 rounded border border-white/30 text-[10px] text-white/80 bg-white/10 backdrop-blur-sm">
            {codecTag}
          </span>
        </div>

        {/* Hover Action Buttons */}
        <div className="mt-4 flex items-center gap-2 h-0 opacity-0 group-hover:h-9 group-hover:opacity-100 group-hover:mt-3 transition-all duration-500 delay-100 overflow-hidden pointer-events-auto">
          <button
            onClick={(e) => { e.stopPropagation(); onPlay(file); }}
            className="cursor-pointer flex-1 bg-white hover:bg-white/80 text-black rounded-sm py-1.5 px-3 flex items-center justify-center gap-1.5 font-bold text-sm transition-colors"
          >
            <Play size={16} className="fill-current" /> Play
          </button>
          
          {onAddToCategory && (
            <button
              onClick={(e) => { e.stopPropagation(); onAddToCategory(file); }}
              className="cursor-pointer w-9 h-9 flex-shrink-0 rounded-full border-2 border-white/40 hover:border-white bg-[#141414]/60 hover:bg-white/10 flex items-center justify-center text-white transition-all"
              title="Add to List"
            >
              <Plus size={18} />
            </button>
          )}

          <button
            onClick={(e) => { e.stopPropagation(); onOpenInfo(file); }}
            className="cursor-pointer w-9 h-9 flex-shrink-0 rounded-full border-2 border-white/40 hover:border-white bg-[#141414]/60 hover:bg-white/10 flex items-center justify-center text-white transition-all ml-auto"
            title="More Info"
          >
            <Info size={16} />
          </button>
        </div>

        {/* Progress Bar (Always visible at very bottom if present) */}
        {progressPercent !== undefined && progressPercent > 0 && (
          <div className="absolute bottom-0 left-0 right-0 h-1 bg-[#404040]">
            <div className="h-full bg-[#E50914]" style={{ width: `${progressPercent}%` }} />
          </div>
        )}
      </div>
    </div>
  );
};
