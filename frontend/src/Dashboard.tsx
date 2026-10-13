import React, { useState, useRef, useEffect, useMemo, useCallback } from "react";
import { 
  uploadFile, 
  downloadFile, 
  deleteFile as apiDeleteFile, 
  listFolder, 
  createFolder as apiCreateFolder, 
  deleteFolder as apiDeleteFolder, 
  renameFolder as apiRenameFolder,
  clearToken,
  getBotPoolStatus,
  getStorageStats,
  StorageStats,
  getToken,
  setFileCinema
} from "./api/client";
import { FileItem, FolderItem, formatFileSize, getFileCategory, getPreviewType } from "./types";
import FilePreview from "./components/FilePreview";
import UploadModal from "./components/UploadModal";
import UploadStatusDropdown from "./components/UploadStatusDropdown";
import { useUploadQueue } from "./hooks/useUploadQueue";
import { VideoThumbnail } from "./components/VideoThumbnail";
import { NetflixVideoCard, getCleanMovieTitle } from "./components/NetflixVideoCard";

export default function Dashboard() {
  const [activeTab, setActiveTab] = useState("home");
  const [files, setFiles] = useState<FileItem[]>([]);
  const [folders, setFolders] = useState<FolderItem[]>([]);
  const [currentFolderId, setCurrentFolderId] = useState<string | null>(null);
  const [pathChain, setPathChain] = useState<FolderItem[]>([]);

  const navigateToFolder = (folder: { id: string | null, name: string } | null) => {
    if (folder && folder.id) {
      setCurrentFolderId(folder.id);
      setPathChain(prev => {
         const idx = prev.findIndex(f => f.id === folder.id);
         if (idx !== -1) return prev.slice(0, idx + 1);
         return [...prev, folder as FolderItem];
      });
    } else {
      setCurrentFolderId(null);
      setPathChain([]);
    }
  };

  // Modals & Popups state
  const [previewFile, setPreviewFile] = useState<FileItem | null>(null);
  const [showUploadModal, setShowUploadModal] = useState(false);
  const [activeMenu, setActiveMenu] = useState<{ file?: FileItem; folder?: FolderItem; isFolder?: boolean; x: number; y: number } | null>(null);
  const [renameTarget, setRenameTarget] = useState<{ item: FileItem | FolderItem; isFolder: boolean; name: string } | null>(null);
  const [detailsTarget, setDetailsTarget] = useState<FileItem | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<{ item: FileItem | FolderItem; isFolder: boolean } | null>(null);
  const [shareTarget, setShareTarget] = useState<FileItem | null>(null);
  const [showNewFolderModal, setShowNewFolderModal] = useState(false);
  const [newFolderName, setNewFolderName] = useState('');
  const [showCreateCategoryModal, setShowCreateCategoryModal] = useState(false);
  const [newCategoryName, setNewCategoryName] = useState('');
  const [showCreateAlbumModal, setShowCreateAlbumModal] = useState(false);
  const [newAlbumName, setNewAlbumName] = useState('');
  const [newAlbumArtist, setNewAlbumArtist] = useState('');
  const [showSettingsModal, setShowSettingsModal] = useState(false);
  const [showMovieInfoModal, setShowMovieInfoModal] = useState(false);
  const [showSortMenu, setShowSortMenu] = useState(false);
  const [selectedMovieForInfo, setSelectedMovieForInfo] = useState<FileItem | null>(null);
  const [warmupEnabled, setWarmupEnabled] = useState(true);

  // Telegram Bot Pool Shard Status & Storage Stats
  const [botPool, setBotPool] = useState<{ total: number; online: number }>({ total: 4, online: 4 });
  const [storageStats, setStorageStats] = useState<StorageStats | null>(null);

  useEffect(() => {
    getBotPoolStatus()
      .then((status) => {
        if (status && typeof status.total === 'number') {
          setBotPool(status);
        }
      })
      .catch(() => {
        setBotPool({ total: 4, online: 4 });
      });

    getStorageStats()
      .then(stats => setStorageStats(stats))
      .catch(console.error);
  }, []);

  // Search & Filter
  const [searchQuery, setSearchQuery] = useState("");
  const [sortBy, setSortBy] = useState<'date' | 'name' | 'size' | 'type'>('date');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');
  const [viewMode, setViewMode] = useState<'grid' | 'list'>('grid');
  const [favorites, setFavorites] = useState<Set<string>>(new Set());

  // Toast Notifications
  const [toasts, setToasts] = useState<Array<{ id: string; message: string; type?: 'success' | 'info' | 'error' }>>([]);
  const showToast = useCallback((message: string, type: 'success' | 'info' | 'error' = 'info') => {
    const id = Date.now().toString() + Math.random();
    setToasts(prev => [...prev, { id, message, type }]);
    setTimeout(() => {
      setToasts(prev => prev.filter(t => t.id !== id));
    }, 3200);
  }, []);

  // Background Upload Queue Manager
  const {
    tasks: uploadTasks,
    startUploads,
    cancelUpload,
    cancelAll: cancelAllUploads,
    clearFinished: clearFinishedUploads,
  } = useUploadQueue({
    onFileUploaded: (uploadedFile) => {
      setFiles((prev) => [uploadedFile, ...prev]);
      showToast(`"${uploadedFile.name}" encrypted & uploaded successfully!`, 'success');
      listFolder(currentFolderId || undefined)
        .then((c) => {
          if (c.files) setFiles(c.files);
        })
        .catch(() => {});
    },
    onError: (err, task) => {
      showToast(`Upload failed for "${task.file.name}": ${err.message}`, 'error');
    },
    onCancel: (task) => {
      showToast(`Cancelled upload for "${task.file.name}"`, 'info');
    },
  });

  const greeting = useMemo(() => {
    const hour = new Date().getHours();
    if (hour < 12) return "Good morning";
    if (hour < 18) return "Good afternoon";
    return "Good evening";
  }, []);

  const activeTransfersCount = useMemo(() => {
    return uploadTasks.filter(t => t.status === 'uploading' || t.status === 'pending').length;
  }, [uploadTasks]);

  // Audio Dock Player state & Music Tab dynamic state
  const [playlist, setPlaylist] = useState<any[]>([]);
  const [currentTrackIndex, setCurrentTrackIndex] = useState(0);
  const audioRef = useRef<HTMLAudioElement>(null);
  const activeTrackIdRef = useRef<string | null>(null);
  const [isPlayingAudio, setIsPlayingAudio] = useState(false);
  const [audioProgressSec, setAudioProgressSec] = useState(0);
  const [trackDurationSec, setTrackDurationSec] = useState<number>(0);
  const [isShuffle, setIsShuffle] = useState(false);
  const [isRepeat, setIsRepeat] = useState(false);
  const [isDockMinimized, setIsDockMinimized] = useState(false);
  const [showQueueDrawer, setShowQueueDrawer] = useState(false);
  const [volume, setVolume] = useState(0.75);
  const [isMuted, setIsMuted] = useState(false);
  const [isSidebarOpen, setIsSidebarOpen] = useState(true);
  const [hasActiveAudioSession, setHasActiveAudioSession] = useState(false);

  // Recently played audio IDs persisted in localStorage
  const [recentlyPlayedIds, setRecentlyPlayedIds] = useState<string[]>(() => {
    try {
      const saved = localStorage.getItem('tgdrive_recent_audio');
      return saved ? JSON.parse(saved) : [];
    } catch {
      return [];
    }
  });

  // Music Tab Filters & Sorting state
  const [musicFilter, setMusicFilter] = useState<'all' | 'recent' | 'queue' | 'folders' | 'favorites'>('all');
  const [musicSearchQuery, setMusicSearchQuery] = useState("");
  const [musicSortBy, setMusicSortBy] = useState<'name' | 'date' | 'duration' | 'size'>('name');
  const [musicSortOrder, setMusicSortOrder] = useState<'asc' | 'desc'>('asc');
  const [showMusicSortMenu, setShowMusicSortMenu] = useState(false);
  const [selectedMusicFolderId, setSelectedMusicFolderId] = useState<string | null>(null);

  // Helper to extract clean artist and track title from audio filename
  const parseAudioInfo = (filename: string, metaArtist?: string, metaTitle?: string) => {
    if (metaTitle && metaTitle.trim()) {
      return {
        title: metaTitle.trim(),
        artist: (metaArtist && metaArtist.trim()) || "Unknown Artist"
      };
    }
    const cleanName = (filename || "").replace(/\.[^/.]+$/, "");
    if (cleanName.includes(" - ")) {
      const parts = cleanName.split(" - ");
      const artist = parts[0].trim();
      const title = parts.slice(1).join(" - ").trim();
      if (artist && title) {
        return { artist, title };
      }
    }
    return {
      artist: (metaArtist && metaArtist.trim()) || "TgDrive Audio",
      title: cleanName || filename || "Audio Track"
    };
  };

  useEffect(() => {
    if (isPlayingAudio) {
      setHasActiveAudioSession(true);
    }
  }, [isPlayingAudio]);

  const currentTrackRaw = playlist[currentTrackIndex] || playlist[0] || null;
  const currentTrack = useMemo(() => {
    if (!currentTrackRaw) {
      return {
        fileId: "",
        rawItem: null as any,
        title: "No track selected",
        artist: "TgDrive Audio",
        durationSec: 0,
        metadataDuration: 0,
        format: "AUDIO",
        cover: undefined as string | undefined
      };
    }
    const fileId = currentTrackRaw.id || currentTrackRaw.fileId || "";
    const rawName = currentTrackRaw.name || currentTrackRaw.title || "Track";
    const parsed = parseAudioInfo(
      rawName,
      currentTrackRaw.metadata?.artist,
      currentTrackRaw.metadata?.title
    );
    const metaDur = currentTrackRaw.metadata?.duration || 0;
    const durSec = trackDurationSec > 0 ? trackDurationSec : (metaDur || 0);
    const format = (rawName.split('.').pop() || "AUDIO").toUpperCase();
    const cover = currentTrackRaw.metadata?.thumbnail_telegram_file_id 
      ? `/api/files/${fileId}/thumbnail?token=${getToken()}` 
      : undefined;

    return {
      fileId,
      rawItem: currentTrackRaw,
      title: parsed.title,
      artist: parsed.artist,
      durationSec: durSec,
      metadataDuration: metaDur,
      format,
      cover
    };
  }, [currentTrackRaw, trackDurationSec]);

  // Record to recently played list
  useEffect(() => {
    if (currentTrack?.fileId && isPlayingAudio) {
      setRecentlyPlayedIds(prev => {
        const next = [currentTrack.fileId, ...prev.filter(id => id !== currentTrack.fileId)].slice(0, 30);
        try { localStorage.setItem('tgdrive_recent_audio', JSON.stringify(next)); } catch {}
        return next;
      });
    }
  }, [currentTrack?.fileId, isPlayingAudio]);

  // Audio queue control handlers
  const handleNextTrack = useCallback(() => {
    if (playlist.length <= 1) {
      if (audioRef.current) {
        audioRef.current.currentTime = 0;
        audioRef.current.play().catch(console.error);
      }
      return;
    }
    if (isShuffle) {
      let nextIdx = currentTrackIndex;
      while (nextIdx === currentTrackIndex && playlist.length > 1) {
        nextIdx = Math.floor(Math.random() * playlist.length);
      }
      setCurrentTrackIndex(nextIdx);
    } else {
      setCurrentTrackIndex(prev => (prev + 1) % playlist.length);
    }
    setAudioProgressSec(0);
    setIsPlayingAudio(true);
  }, [playlist.length, currentTrackIndex, isShuffle]);

  const handlePrevTrack = useCallback(() => {
    if (audioRef.current && audioRef.current.currentTime > 3) {
      audioRef.current.currentTime = 0;
      setAudioProgressSec(0);
      return;
    }
    if (playlist.length <= 1) {
      if (audioRef.current) {
        audioRef.current.currentTime = 0;
      }
      return;
    }
    if (isShuffle) {
      let prevIdx = currentTrackIndex;
      while (prevIdx === currentTrackIndex && playlist.length > 1) {
        prevIdx = Math.floor(Math.random() * playlist.length);
      }
      setCurrentTrackIndex(prevIdx);
    } else {
      setCurrentTrackIndex(prev => (prev - 1 + playlist.length) % playlist.length);
    }
    setAudioProgressSec(0);
    setIsPlayingAudio(true);
  }, [playlist.length, currentTrackIndex, isShuffle]);

  const handleAddToQueue = (file: FileItem) => {
    setPlaylist(prev => {
      if (prev.some(f => (f.id || f.fileId) === file.id)) {
        showToast(`"${file.name}" is already in queue`, "info");
        return prev;
      }
      showToast(`Added "${file.name}" to queue`, "success");
      return [...prev, file];
    });
    setHasActiveAudioSession(true);
  };

  const handlePlayNext = (file: FileItem) => {
    setPlaylist(prev => {
      const filtered = prev.filter(f => (f.id || f.fileId) !== file.id);
      const insertIdx = Math.min(filtered.length, currentTrackIndex + 1);
      const nextList = [...filtered.slice(0, insertIdx), file, ...filtered.slice(insertIdx)];
      showToast(`"${file.name}" will play next`, "success");
      return nextList;
    });
    setHasActiveAudioSession(true);
  };

  const handleRemoveFromQueue = (idx: number) => {
    if (playlist.length <= 1) {
      setPlaylist([]);
      setIsPlayingAudio(false);
      setHasActiveAudioSession(false);
      activeTrackIdRef.current = null;
      setShowQueueDrawer(false);
      showToast("Queue cleared", "info");
      return;
    }
    const nextList = playlist.filter((_, i) => i !== idx);
    setPlaylist(nextList);
    if (idx === currentTrackIndex) {
      setCurrentTrackIndex(Math.min(idx, nextList.length - 1));
      setAudioProgressSec(0);
    } else if (idx < currentTrackIndex) {
      setCurrentTrackIndex(currentTrackIndex - 1);
    }
    showToast("Removed track from queue", "info");
  };

  const handleRemoveFileFromQueue = (fileId: string) => {
    const idx = playlist.findIndex(f => (f.id || f.fileId) === fileId);
    if (idx !== -1) {
      handleRemoveFromQueue(idx);
    } else {
      showToast("Track is not in queue", "info");
    }
  };

  const handleClearQueue = () => {
    setPlaylist([]);
    setIsPlayingAudio(false);
    setAudioProgressSec(0);
    activeTrackIdRef.current = null;
    setShowQueueDrawer(false);
    showToast("Queue cleared", "info");
  };

  // Load from real backend if connected
  useEffect(() => {
    listFolder(currentFolderId || undefined)
      .then(contents => {
        if (contents.files) setFiles(contents.files);
        if (contents.folders) setFolders(contents.folders);
      })
      .catch(console.error);
  }, [currentFolderId]);

  // Real audio playback integration
  useEffect(() => {
    if (audioRef.current && currentTrack.fileId) {
      if (isPlayingAudio) {
        audioRef.current.play().catch(err => {
          if (err.name !== 'AbortError') {
            console.warn("Autoplay prevented:", err);
            setIsPlayingAudio(false);
          }
        });
      } else {
        if (!audioRef.current.paused) {
          audioRef.current.pause();
        }
      }
    }
  }, [isPlayingAudio, currentTrackIndex, currentTrack.fileId]);

  useEffect(() => {
    if (audioRef.current) {
      audioRef.current.volume = volume;
      audioRef.current.muted = isMuted;
    }
  }, [volume, isMuted]);

  // Handle track changing (only reset seek position when switching to a different track)
  useEffect(() => {
    const curFileId = currentTrack.fileId;
    if (!curFileId) return;

    if (activeTrackIdRef.current !== curFileId) {
      activeTrackIdRef.current = curFileId;
      setAudioProgressSec(0);
      const rawDur = playlist[currentTrackIndex]?.metadata?.duration;
      setTrackDurationSec(rawDur || 0);
      if (audioRef.current) {
        audioRef.current.currentTime = 0;
      }
    }
  }, [currentTrack.fileId, currentTrackIndex, playlist]);

  // Helper to find file by name or id
  const findFileByName = useCallback((name: string): FileItem => {
    return files.find(f => f.name.toLowerCase() === name.toLowerCase()) || files[0] || { id: 'dummy', name: name, size: 0, mime_type: 'unknown', folder_id: null, encryption_salt: '', chunk_count: 0, created_at: new Date().toISOString() };
  }, [files]);

  // Video files collection for General Library vs Cinema & Stream
  const allVideoFiles = useMemo(() => {
    return files.filter(f => f.mime_type.startsWith('video/') || ['.mp4', '.mkv', '.webm', '.mov', '.avi'].some(ext => f.name.toLowerCase().endsWith(ext)));
  }, [files]);

  const cinemaVideos = useMemo(() => {
    return files.filter(f => Boolean(f.is_cinema) && (f.mime_type.startsWith('video/') || ['.mp4', '.mkv', '.webm', '.mov', '.avi'].some(ext => f.name.toLowerCase().endsWith(ext))));
  }, [files]);

  const [selectedHeroId, setSelectedHeroId] = useState<string | null>(null);
  const [cinemaSearchQuery, setCinemaSearchQuery] = useState("");
  const [cinemaViewMode, setCinemaViewMode] = useState<'shelves' | 'grid'>('shelves');
  const [showAddCinemaModal, setShowAddCinemaModal] = useState(false);
  const [cinemaModalSearch, setCinemaModalSearch] = useState("");

  const heroMovie = useMemo(() => {
    if (selectedHeroId) {
      const found = cinemaVideos.find(f => f.id === selectedHeroId);
      if (found) return found;
    }
    return cinemaVideos[0] || null;
  }, [cinemaVideos, selectedHeroId]);

  const filteredCinemaVideos = useMemo(() => {
    if (!cinemaSearchQuery.trim()) return cinemaVideos;
    const q = cinemaSearchQuery.toLowerCase().trim();
    return cinemaVideos.filter(f => f.name.toLowerCase().includes(q) || getCleanMovieTitle(f.name).toLowerCase().includes(q));
  }, [cinemaVideos, cinemaSearchQuery]);

  const handleToggleCinema = async (file: FileItem) => {
    const newStatus = !file.is_cinema;
    setFiles(prev => prev.map(f => f.id === file.id ? { ...f, is_cinema: newStatus } : f));
    try {
      await setFileCinema(file.id, newStatus);
      showToast(newStatus ? `Added "${file.name}" to Cinema` : `Removed "${file.name}" from Cinema`, "success");
    } catch {
      setFiles(prev => prev.map(f => f.id === file.id ? { ...f, is_cinema: !newStatus } : f));
      showToast("Failed to update Cinema status", "error");
    }
  };

  // Actions
  const handlePreview = (file: FileItem) => {
    if (getPreviewType(file.mime_type, file.name) === 'audio') {
      const allAudioFiles = files.filter(f => getPreviewType(f.mime_type, f.name) === 'audio');
      // Set the entire current audio files collection as the playlist
      const targetList = allAudioFiles.length > 0 ? allAudioFiles : [file];
      setPlaylist(targetList);
      const index = targetList.findIndex(f => f.id === file.id);
      setCurrentTrackIndex(index >= 0 ? index : 0);
      setAudioProgressSec(0);
      setIsPlayingAudio(true);
      setIsDockMinimized(false);
      setHasActiveAudioSession(true);
      setPreviewFile(null);
      return;
    }
    setPreviewFile(file);
  };

  const handleDockToBottomPlayer = (file: FileItem) => {
    handlePreview(file);
    showToast(`Now playing "${file.name}" in bottom floating player`, "success");
  };

  const handleDownload = async (file: FileItem) => {
    showToast(`Downloading "${file.name}"...`, "info");
    try {
      await downloadFile(file.id, file.name);
    } catch {
      // Fallback direct anchor
      const a = document.createElement('a');
      a.href = `/api/files/${file.id}/download?token=${getToken()}`;
      a.download = file.name;
      a.click();
    }
  };

  const handleShare = (file: FileItem) => {
    setShareTarget(file);
    const link = `${window.location.origin}/api/files/${file.id}/stream?token=${getToken()}`;
    navigator.clipboard.writeText(link).catch(() => {});
    showToast(`Share link for "${file.name}" copied to clipboard!`, "success");
  };

  const handleToggleFavorite = (fileId: string) => {
    setFavorites(prev => {
      const next = new Set(prev);
      if (next.has(fileId)) {
        next.delete(fileId);
        showToast("Removed from favorites", "info");
      } else {
        next.add(fileId);
        showToast("Added to favorites!", "success");
      }
      return next;
    });
  };

  const handleOpenMenu = (e: React.MouseEvent, file: FileItem) => {
    e.preventDefault();
    e.stopPropagation();
    setActiveMenu({ file, isFolder: false, x: e.clientX, y: e.clientY });
  };

  const handleOpenFolderMenu = (e: React.MouseEvent, folder: FolderItem) => {
    e.preventDefault();
    e.stopPropagation();
    setActiveMenu({ folder, isFolder: true, x: e.clientX, y: e.clientY });
  };

  const menuCallbackRef = useCallback((node: HTMLDivElement | null) => {
    if (!node || !activeMenu) return;
    const rect = node.getBoundingClientRect();
    const padding = 12;
    let x = activeMenu.x;
    let y = activeMenu.y;

    if (x + rect.width > window.innerWidth - padding) {
      x = Math.max(padding, window.innerWidth - rect.width - padding);
    }
    if (y + rect.height > window.innerHeight - padding) {
      if (activeMenu.y - rect.height >= padding) {
        y = activeMenu.y - rect.height;
      } else {
        y = Math.max(padding, window.innerHeight - rect.height - padding);
      }
    }
    x = Math.max(padding, x);
    y = Math.max(padding, y);

    node.style.left = `${x}px`;
    node.style.top = `${y}px`;
  }, [activeMenu]);

  const handleRenameSubmit = async () => {
    if (!renameTarget || !renameTarget.name.trim()) return;
    const oldName = renameTarget.item.name;
    const newName = renameTarget.name.trim();

    if (renameTarget.isFolder) {
      setFolders(folders.map(f => f.id === renameTarget.item.id ? { ...f, name: newName } : f));
      try { await apiRenameFolder(renameTarget.item.id, newName); } catch {}
    } else {
      setFiles(files.map(f => f.id === renameTarget.item.id ? { ...f, name: newName } : f));
    }
    showToast(`Renamed "${oldName}" to "${newName}"`, "success");
    setRenameTarget(null);
  };

  const handleDeleteConfirm = async () => {
    if (!deleteTarget) return;
    const targetName = deleteTarget.item.name;

    if (deleteTarget.isFolder) {
      setFolders(folders.filter(f => f.id !== deleteTarget.item.id));
      try { await apiDeleteFolder(deleteTarget.item.id); } catch {}
    } else {
      setFiles(files.filter(f => f.id !== deleteTarget.item.id));
      try { await apiDeleteFile(deleteTarget.item.id); } catch {}
    }
    showToast(`Deleted "${targetName}" from Telegram cloud`, "info");
    setDeleteTarget(null);
  };

  const handleNewFolderSubmit = async () => {
    if (!newFolderName.trim()) return;
    const name = newFolderName.trim();
    const newFol: FolderItem = {
      id: "fol-" + Date.now(),
      name,
      parent_id: currentFolderId,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    };
    setFolders([...folders, newFol]);
    try { await apiCreateFolder(name, currentFolderId || undefined); } catch {}
    showToast(`Created folder "${name}"`, "success");
    setNewFolderName('');
    setShowNewFolderModal(false);
  };

  const handleCreateCategorySubmit = () => {
    if (!newCategoryName.trim()) return;
    setVideoCategories([...videoCategories, { id: 'cat-' + Date.now(), title: newCategoryName.trim() }]);
    showToast(`Added video category "${newCategoryName.trim()}"`, "success");
    setNewCategoryName('');
    setShowCreateCategoryModal(false);
  };

  const handleCreateAlbumSubmit = () => {
    if (!newAlbumName.trim()) return;
    setMusicAlbums([...musicAlbums, { id: 'alb-' + Date.now(), title: newAlbumName.trim() }]);
    showToast(`Created music album "${newAlbumName.trim()}"`, "success");
    setNewAlbumName('');
    setNewAlbumArtist('');
    setShowCreateAlbumModal(false);
  };

  const handleLogout = () => {
    clearToken();
    window.location.reload();
  };

  const [videoCategories, setVideoCategories] = useState([
    { id: 'all', title: 'All Videos' },
    { id: 'fav', title: 'Favorites' }
  ]);
  const [musicAlbums, setMusicAlbums] = useState([
    { id: 'fav', title: 'Favorites' },
    { id: 'liked', title: 'Liked Albums' }
  ]);

  const handleRemoveVideoCategory = (id: string) => {
    setVideoCategories(videoCategories.filter(c => c.id !== id));
    showToast("Category removed", "info");
  };

  const handleRemoveMusicAlbum = (id: string) => {
    setMusicAlbums(musicAlbums.filter(a => a.id !== id));
    showToast("Album removed", "info");
  };

  // Compute breadcrumb path for currentFolderId
  const breadcrumbs = useMemo(() => {
    const path: {id: string | null, name: string}[] = [{id: null, name: 'TgDrive'}];
    return [...path, ...pathChain.map(f => ({ id: f.id, name: f.name }))];
  }, [pathChain]);

  const visibleFolders = useMemo(() => {
    if (activeTab !== 'all-files') return [];
    return folders.filter(f => (f.parent_id || null) === (currentFolderId || null));
  }, [folders, currentFolderId, activeTab]);

  // Filtered files for the current library tab
  const filteredFiles = useMemo(() => {
    let result = files;

    // Filter by tab
    if (activeTab === 'videos') {
      result = result.filter(f => f.mime_type.startsWith('video/') || ['.mp4', '.mkv', '.webm', '.mov', '.avi'].some(ext => f.name.toLowerCase().endsWith(ext)));
    } else if (activeTab === 'audio') {
      result = result.filter(f => f.mime_type.startsWith('audio/') || ['.mp3', '.flac', '.wav', '.m4a', '.aac'].some(ext => f.name.toLowerCase().endsWith(ext)));
    } else if (activeTab === 'photos') {
      result = result.filter(f => f.mime_type.startsWith('image/') || ['.jpg', '.jpeg', '.png', '.webp', '.gif', '.svg'].some(ext => f.name.toLowerCase().endsWith(ext)));
    } else if (activeTab === 'documents') {
      result = result.filter(f => f.mime_type.includes('pdf') || f.mime_type.includes('document') || ['.pdf', '.doc', '.docx', '.txt', '.md', '.csv', '.xlsx'].some(ext => f.name.toLowerCase().endsWith(ext)));
    } else if (activeTab === 'archives') {
      result = result.filter(f => f.mime_type.includes('zip') || f.mime_type.includes('tar') || ['.zip', '.tar', '.gz', '.7z', '.rar'].some(ext => f.name.toLowerCase().endsWith(ext)));
    } else if (activeTab === 'others') {
      const knownExts = ['.mp4', '.mkv', '.webm', '.mp3', '.flac', '.wav', '.jpg', '.jpeg', '.png', '.webp', '.pdf', '.docx', '.xlsx', '.zip', '.tar', '.7z'];
      result = result.filter(f => !knownExts.some(ext => f.name.toLowerCase().endsWith(ext)) && !f.mime_type.startsWith('image/') && !f.mime_type.startsWith('video/') && !f.mime_type.startsWith('audio/'));
    } else if (activeTab === 'all-files') {
      result = result.filter(f => (f.folder_id || null) === (currentFolderId || null));
    }

    // Filter by search query
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      result = result.filter(f => f.name.toLowerCase().includes(q));
    }

    // Sort
    return [...result].sort((a, b) => {
      let cmp = 0;
      if (sortBy === 'name') cmp = a.name.localeCompare(b.name);
      else if (sortBy === 'size') cmp = a.size - b.size;
      else if (sortBy === 'date') cmp = new Date(a.created_at).getTime() - new Date(b.created_at).getTime();
      else if (sortBy === 'type') {
        const extA = a.name.split('.').pop() || '';
        const extB = b.name.split('.').pop() || '';
        cmp = extA.localeCompare(extB);
        if (cmp === 0) cmp = a.name.localeCompare(b.name); // fallback to name
      }
      return sortOrder === 'desc' ? -cmp : cmp;
    });
  }, [files, activeTab, searchQuery, sortBy, sortOrder]);

  // All audio files in the entire library
  const allAudioFiles = useMemo(() => {
    return files.filter(f => getPreviewType(f.mime_type, f.name) === 'audio');
  }, [files]);

  // Audio files grouped by folder
  const audioFolders = useMemo(() => {
    const map = new Map<string, { folder: FolderItem | null; files: FileItem[] }>();
    allAudioFiles.forEach(f => {
      const folderKey = f.folder_id || 'root';
      const folderObj = f.folder_id ? (folders.find(fol => fol.id === f.folder_id) || null) : null;
      if (!map.has(folderKey)) {
        map.set(folderKey, { folder: folderObj, files: [] });
      }
      map.get(folderKey)!.files.push(f);
    });
    return Array.from(map.entries()).map(([key, data]) => ({
      id: key,
      name: data.folder ? data.folder.name : 'Root Folder',
      count: data.files.length,
      files: data.files
    }));
  }, [allAudioFiles, folders]);

  // Recently played audio files in order
  const recentAudioFiles = useMemo(() => {
    const map = new Map(allAudioFiles.map(f => [f.id, f]));
    return recentlyPlayedIds.map(id => map.get(id)).filter(Boolean) as FileItem[];
  }, [allAudioFiles, recentlyPlayedIds]);

  // Favorite audio files
  const favoriteAudioFiles = useMemo(() => {
    return allAudioFiles.filter(f => favorites.has(f.id));
  }, [allAudioFiles, favorites]);

  // Displayed audio tracks filtered and sorted for Music Tab
  const displayedMusicTracks = useMemo(() => {
    let result: FileItem[] = [];
    if (musicFilter === 'recent') {
      result = [...recentAudioFiles];
    } else if (musicFilter === 'queue') {
      result = playlist.map(item => {
        const found = allAudioFiles.find(f => f.id === (item.id || item.fileId));
        return found || item;
      });
    } else if (musicFilter === 'favorites') {
      result = [...favoriteAudioFiles];
    } else if (musicFilter === 'folders' && selectedMusicFolderId) {
      const fGroup = audioFolders.find(g => g.id === selectedMusicFolderId);
      result = fGroup ? [...fGroup.files] : [];
    } else {
      result = [...allAudioFiles];
    }

    // Search query filter
    if (musicSearchQuery.trim()) {
      const q = musicSearchQuery.toLowerCase().trim();
      result = result.filter(f => {
        const parsed = parseAudioInfo(f.name, f.metadata?.artist, f.metadata?.title);
        return f.name.toLowerCase().includes(q) ||
               parsed.title.toLowerCase().includes(q) ||
               parsed.artist.toLowerCase().includes(q);
      });
    }

    // Sort
    result.sort((a, b) => {
      let cmp = 0;
      if (musicSortBy === 'name') {
        const nameA = (a.metadata?.title || a.name).toLowerCase();
        const nameB = (b.metadata?.title || b.name).toLowerCase();
        cmp = nameA.localeCompare(nameB);
      } else if (musicSortBy === 'date') {
        cmp = new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
      } else if (musicSortBy === 'duration') {
        const durA = a.metadata?.duration || 0;
        const durB = b.metadata?.duration || 0;
        cmp = durB - durA;
      } else if (musicSortBy === 'size') {
        cmp = b.size - a.size;
      }
      return musicSortOrder === 'asc' ? cmp : -cmp;
    });

    return result;
  }, [allAudioFiles, recentAudioFiles, playlist, favoriteAudioFiles, musicFilter, selectedMusicFolderId, audioFolders, musicSearchQuery, musicSortBy, musicSortOrder]);

  const fileInputRef = useRef<HTMLInputElement>(null);
  const [isUploading, setIsUploading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState(0);

  const handleUploadClick = () => {
    setShowUploadModal(true);
  };

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    if (!e.target.files || e.target.files.length === 0) return;
    const file = e.target.files[0];
    try {
      setIsUploading(true);
      setUploadProgress(0);
      const uploadedItem = await uploadFile(file, currentFolderId || undefined, (progress) => {
        setUploadProgress(progress);
      });
      setFiles(prev => [uploadedItem, ...prev]);
      showToast(`"${file.name}" encrypted & uploaded successfully!`, "success");
    } catch (err: any) {
      console.error("Upload failed", err);
      showToast(`Upload failed: ${err.message}`, "error");
    } finally {
      setIsUploading(false);
      setUploadProgress(0);
      if (fileInputRef.current) fileInputRef.current.value = "";
    }
  };

  const formatBytes = (bytes: number) => {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  };

  const formatSec = (s: number) => {
    const mins = Math.floor(s / 60);
    const secs = Math.floor(s % 60);
    return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
  };

  const formatDate = (dateStr: string) => {
    try {
      const d = new Date(dateStr);
      return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
    } catch {
      return dateStr;
    }
  };

  const getFileIcon = (file: FileItem) => {
    const mime = (file.mime_type || '').toLowerCase();
    const name = (file.name || '').toLowerCase();
    if (mime.startsWith('image/')) return 'image';
    if (mime.startsWith('video/')) return 'movie';
    if (mime.startsWith('audio/')) return 'headphones';
    if (mime.includes('pdf')) return 'picture_as_pdf';
    if (name.endsWith('.md') || name.endsWith('.txt')) return 'description';
    if (name.endsWith('.csv') || name.endsWith('.xlsx')) return 'table_chart';
    if (name.endsWith('.zip') || name.endsWith('.tar') || name.endsWith('.gz') || name.endsWith('.7z')) return 'folder_zip';
    if (name.endsWith('.iso') || name.endsWith('.bin') || name.endsWith('.dmg')) return 'disc_full';
    return 'draft';
  };

  const getNavClass = (path: string) => {
    return activeTab === path
      ? "flex items-center gap-space-sm px-space-sm py-space-xs transition-all bg-primary text-on-primary font-semibold rounded-xl shadow-[0_0_12px_rgba(217,155,78,0.25)]"
      : "flex items-center gap-space-sm px-space-sm py-space-xs rounded-xl text-on-surface-variant hover:bg-surface-container-high hover:text-on-surface transition-all";
  };

  const getEntClass = (path: string) => {
    return activeTab === path
      ? "flex items-center justify-between px-space-sm py-space-xs rounded-xl bg-primary text-on-primary font-semibold shadow-[0_0_12px_rgba(217,155,78,0.25)] transition-all group"
      : "flex items-center justify-between px-space-sm py-space-xs rounded-xl text-on-surface-variant hover:bg-surface-container-high hover:text-on-surface transition-all group";
  };

  const toGB = (bytes: number) => (bytes / (1024 ** 3)).toFixed(1);
  const toPct = (bytes: number, total: number) => total > 0 ? ((bytes / total) * 100).toFixed(1) + '%' : '0%';

  const totalBytes = storageStats ? storageStats.total_bytes : 0;
  const totalGB = storageStats ? toGB(storageStats.total_bytes) : "0.0";
  const totalFilesStr = storageStats ? storageStats.total_files.toLocaleString() : "0";

  const videoGB = storageStats ? toGB(storageStats.video_bytes) : "0.0";
  const videoPct = storageStats ? toPct(storageStats.video_bytes, totalBytes) : "0%";
  const photosGB = storageStats ? toGB(storageStats.image_bytes) : "0.0";
  const photosPct = storageStats ? toPct(storageStats.image_bytes, totalBytes) : "0%";
  const audioGB = storageStats ? toGB(storageStats.audio_bytes) : "0.0";
  const audioPct = storageStats ? toPct(storageStats.audio_bytes, totalBytes) : "0%";
  const docsGB = storageStats ? toGB(storageStats.document_bytes) : "0.0";
  const docsPct = storageStats ? toPct(storageStats.document_bytes, totalBytes) : "0%";
  const archivesGB = storageStats ? toGB(storageStats.archive_bytes) : "0.0";
  const archivesPct = storageStats ? toPct(storageStats.archive_bytes, totalBytes) : "0%";
  const othersGB = storageStats ? toGB(storageStats.other_bytes) : "0.0";
  const othersPct = storageStats ? toPct(storageStats.other_bytes, totalBytes) : "0%";

  return (
    <div className="bg-surface font-body-md text-on-surface antialiased overflow-x-hidden">
      <aside className={`fixed left-0 top-0 h-full w-72 bg-surface-container-low/95 backdrop-blur-2xl z-50 flex flex-col justify-between shadow-[0_1px_8px_rgba(0,0,0,0.5)] border-r border-outline-variant/50 transition-transform duration-300 ${isSidebarOpen ? 'translate-x-0' : '-translate-x-full'}`}>
        <div className="flex flex-col flex-1 overflow-y-auto px-space-md py-space-lg">
          <div className="flex items-center justify-between px-space-sm mb-space-lg">
            <div className="flex items-center gap-space-sm">
              <div className="w-9 h-9 rounded-xl bg-primary-container/20 border border-primary/20 flex items-center justify-center text-primary">
                <span className="material-symbols-outlined text-2xl">cloud</span>
              </div>
              <div className="flex flex-col">
                <span className="font-headline-sm text-headline-sm text-on-surface tracking-tight font-bold">
                  TgDrive
                </span>
                <span className="font-label-mono-sm text-label-mono-sm text-on-surface-variant/80">
                  v2.4 Core OS
                </span>
              </div>
            </div>
            <button 
              onClick={() => setIsSidebarOpen(false)}
              className="p-1 text-outline hover:text-on-surface transition-colors cursor-pointer"
              title="Collapse sidebar"
            >
              <span className="material-symbols-outlined">menu_open</span>
            </button>
          </div>
          <div className="space-y-space-md">
            <nav
              className="space-y-space-xs"
              data-active-classes="bg-primary-container text-on-primary-container font-semibold rounded-xl"
            >
              <div className="px-space-sm pb-space-xs font-label-mono-sm text-label-mono-sm uppercase text-on-surface-variant/70 tracking-wider">
                Navigation
              </div>
              <a
                className={getNavClass("home")}
                onClick={(e) => {
                  e.preventDefault();
                  setActiveTab("home");
                }}
                data-path="home"
                href="#"
              >
                <span className="material-symbols-outlined text-[20px]">home</span>
                <span className="font-body-md text-body-md">Home</span>
              </a>

            </nav>
            <div className="pt-space-xs">
              <div className="flex items-center justify-between px-space-sm pb-space-xs">
                <span className="font-label-mono-sm text-label-mono-sm uppercase text-on-surface-variant/70 tracking-wider">
                  Entertainment
                </span>
                <span className="font-label-mono-sm text-label-mono-sm px-1.5 py-0.5 rounded-full bg-surface-container-high text-primary border border-outline-variant/50">
                  Stream Hub
                </span>
              </div>
              <nav
                className="space-y-space-xs"
                data-active-classes="bg-primary text-on-primary font-semibold rounded-xl"
              >
                <a
                  className={getNavClass("cinema-and-stream")}
                  onClick={(e) => {
                    e.preventDefault();
                    setActiveTab("cinema-and-stream");
                  }}
                  data-path="cinema-and-stream"
                  href="#"
                >
                  <span className="material-symbols-outlined text-[20px]">movie</span>
                  <span className="font-body-md text-body-md">
                    Cinema &amp; Stream
                  </span>
                </a>
                <a
                  className={getNavClass("music-and-audio")}
                  onClick={(e) => {
                    e.preventDefault();
                    setActiveTab("music-and-audio");
                  }}
                  data-path="music-and-audio"
                  href="#"
                >
                  <span className="material-symbols-outlined text-[20px]">headphones</span>
                  <span className="font-body-md text-body-md">
                    Music &amp; Audio
                  </span>
                </a>
              </nav>
            </div>
            <div className="pt-space-xs">
              <div className="px-space-sm pb-space-xs font-label-mono-sm text-label-mono-sm uppercase text-on-surface-variant/70 tracking-wider">
                Library
              </div>
              <nav
                className="space-y-space-xs"
                data-active-classes="bg-primary text-on-primary font-semibold rounded-xl"
              >
                <a
                  className={getNavClass("all-files")}
                  onClick={(e) => {
                    e.preventDefault();
                    setActiveTab("all-files");
                  }}
                  data-path="all-files"
                  href="#"
                >
                  <span className="material-symbols-outlined text-[20px]">folder</span>
                  <span className="font-body-md text-body-md">All Files</span>
                </a>
                <a
                  className={getNavClass("videos")}
                  onClick={(e) => {
                    e.preventDefault();
                    setActiveTab("videos");
                  }}
                  data-path="videos"
                  href="#"
                >
                  <span className="material-symbols-outlined text-[20px]">videocam</span>
                  <span className="font-body-md text-body-md">Videos</span>
                </a>
                <a
                  className={getNavClass("audio")}
                  onClick={(e) => {
                    e.preventDefault();
                    setActiveTab("audio");
                  }}
                  data-path="audio"
                  href="#"
                >
                  <span className="material-symbols-outlined text-[20px]">album</span>
                  <span className="font-body-md text-body-md">Audio</span>
                </a>
                <a
                  className={getNavClass("photos")}
                  onClick={(e) => {
                    e.preventDefault();
                    setActiveTab("photos");
                  }}
                  data-path="photos"
                  href="#"
                >
                  <span className="material-symbols-outlined text-[20px]">image</span>
                  <span className="font-body-md text-body-md">Photos</span>
                </a>
                <a
                  className={getNavClass("documents")}
                  onClick={(e) => {
                    e.preventDefault();
                    setActiveTab("documents");
                  }}
                  data-path="documents"
                  href="#"
                >
                  <span className="material-symbols-outlined text-[20px]">description</span>
                  <span className="font-body-md text-body-md">Documents</span>
                </a>

                <a
                  className={getNavClass("archives")}
                  onClick={(e) => {
                    e.preventDefault();
                    setActiveTab("archives");
                  }}
                  data-path="archives"
                  href="#"
                >
                  <span className="material-symbols-outlined text-[20px]">folder_zip</span>
                  <span className="font-body-md text-body-md">Archives</span>
                </a>
                <a
                  className={getNavClass("others")}
                  onClick={(e) => {
                    e.preventDefault();
                    setActiveTab("others");
                  }}
                  data-path="others"
                  href="#"
                >
                  <span className="material-symbols-outlined text-[20px]">category</span>
                  <span className="font-body-md text-body-md">Others</span>
                </a>
              </nav>
            </div>
          </div>
        </div>
        <div className="px-space-md py-space-sm bg-surface-container-lowest border-t border-outline-variant/40 flex flex-col justify-center">
          <div className="flex items-center justify-between">
            <div className="flex items-baseline gap-1">
              <span className="text-lg font-bold text-on-surface tracking-tight leading-none">{totalGB}</span>
              <span className="font-label-mono-sm text-[10px] text-primary ml-0.5">GB</span>
              <span className="font-label-mono-sm text-[10px] text-outline">used</span>
            </div>
            <div className="flex items-center gap-1">
              <span className="material-symbols-outlined text-[12px] text-outline">folder</span>
              <span className="font-label-mono-sm text-[10px] text-outline">{totalFilesStr}</span>
            </div>
          </div>
        </div>
      </aside>
      <div className={`transition-all duration-300 ${isSidebarOpen ? 'lg:pl-72 pl-0' : 'pl-0'}`}>
        <header className={`fixed top-0 right-0 h-16 bg-surface/85 backdrop-blur-xl z-40 flex items-center justify-between px-space-lg shadow-[0_1px_8px_rgba(0,0,0,0.4)] border-b border-outline-variant/40 transition-all duration-300 ${isSidebarOpen ? 'lg:left-72 left-0' : 'left-0'}`}>
          <div className="flex items-center gap-space-sm text-on-surface-variant font-body-sm text-body-sm">
            {!isSidebarOpen && (
              <button 
                onClick={() => setIsSidebarOpen(true)}
                className="p-1.5 mr-2 text-on-surface hover:bg-surface-container-high rounded-lg transition-colors cursor-pointer"
                title="Expand sidebar"
              >
                <span className="material-symbols-outlined">menu</span>
              </button>
            )}
            <span className="font-label-mono-md text-label-mono-md text-primary font-semibold">
              TgDrive
            </span>
            {(() => {
              if (activeTab !== 'all-files') {
                return (
                  <>
                    <span className="text-outline-variant">/</span>
                    <span className="text-on-surface font-body-md text-body-md capitalize">
                      {activeTab.replace(/-/g, ' ')}
                    </span>
                  </>
                );
              }
              const crumbs = pathChain;
              return (
                <>
                  <span className="text-outline-variant">/</span>
                  <span 
                    className={`font-body-md text-body-md capitalize transition-colors ${crumbs.length === 0 ? 'text-on-surface' : 'text-on-surface-variant hover:text-primary cursor-pointer'}`}
                    onClick={() => navigateToFolder(null)}
                  >
                    All files
                  </span>
                  {crumbs.map((crumb, idx) => {
                    const isLast = idx === crumbs.length - 1;
                    return (
                      <React.Fragment key={crumb.id}>
                        <span className="text-outline-variant">/</span>
                        <span 
                          className={`font-body-md text-body-md transition-colors ${isLast ? 'text-on-surface' : 'text-on-surface-variant hover:text-primary cursor-pointer'}`}
                          onClick={isLast ? undefined : () => navigateToFolder(crumb)}
                        >
                          {crumb.name}
                        </span>
                      </React.Fragment>
                    );
                  })}
                </>
              );
            })()}
          </div>
          <div className="flex-1 max-w-xl mx-space-lg">
            <div className="relative flex items-center">
              <span className="material-symbols-outlined absolute left-3 text-outline text-[20px]">search</span>
              <input
                className="w-full bg-surface-container-low text-on-surface placeholder:text-outline text-body-sm font-body-sm rounded-xl pl-10 pr-12 py-2 border border-outline-variant/30 focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary transition-all"
                placeholder="Search files, videos, music..."
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
              {searchQuery ? (
                <button
                  onClick={() => setSearchQuery("")}
                  className="absolute right-3 text-outline hover:text-on-surface transition-colors"
                  title="Clear search"
                >
                  <span className="material-symbols-outlined text-[16px]">close</span>
                </button>
              ) : (
                <kbd className="absolute right-3 font-label-mono-sm text-label-mono-sm bg-surface-container-high px-1.5 py-0.5 rounded text-on-surface-variant border border-outline-variant/40">
                  ⌘K
                </kbd>
              )}
            </div>
          </div>
          <div className="flex items-center gap-space-md">
            {/* Bots Pool Status: (grey bot, total no. of bots), (green light, no. of bots online) */}
            <div 
              className="hidden sm:flex items-center gap-2.5 px-3 py-1.5 rounded-full border border-outline-variant/40 bg-surface-container-low/90 backdrop-blur-md shadow-sm select-none"
              title={`Telegram Shard Bot Pool: ${botPool.online} of ${botPool.total} bots online and ready`}
            >
              {/* (grey bot, total no. of bots) */}
              <div className="flex items-center gap-1.5 text-zinc-400">
                <span className="material-symbols-outlined text-[17px] text-zinc-400">smart_toy</span>
                <span className="font-label-mono-sm text-xs font-semibold text-zinc-300">
                  {botPool.total} {botPool.total === 1 ? 'Bot' : 'Bots'}
                </span>
              </div>

              {/* Separator */}
              <span className="w-px h-3.5 bg-outline-variant/40" />

              {/* (green light, no. of bots online) */}
              <div className="flex items-center gap-1.5">
                <span className="relative flex h-2 w-2 items-center justify-center">
                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                  <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.9)]"></span>
                </span>
                <span className="font-label-mono-sm text-xs font-bold text-emerald-400">
                  {botPool.online} Online
                </span>
              </div>
            </div>

            {/* Upload Status Dropdown */}
            <UploadStatusDropdown
              tasks={uploadTasks}
              onCancel={cancelUpload}
              onCancelAll={cancelAllUploads}
              onClearFinished={clearFinishedUploads}
              onOpenUploadModal={() => setShowUploadModal(true)}
            />

            {/* Upload & Encrypt Button */}
            <button
              className="flex items-center gap-1.5 bg-primary-container hover:bg-primary text-on-primary-container font-body-md text-body-md font-semibold px-3.5 py-2 rounded-xl transition-all shadow-[0_0_12px_rgba(217,155,78,0.25)] cursor-pointer"
              type="button"
              onClick={handleUploadClick}
            >
              <span className="material-symbols-outlined text-[18px]">add</span>
              <span>
                Upload &amp; Encrypt
              </span>
            </button>

            {/* Dedicated Sign Out Button */}
            <button
              type="button"
              onClick={handleLogout}
              className="flex items-center gap-1.5 px-3 py-2 rounded-xl text-xs font-semibold text-on-surface-variant hover:text-rose-400 hover:bg-rose-500/10 border border-outline-variant/40 hover:border-rose-500/30 transition-all cursor-pointer shadow-sm"
              title="Disconnect session & sign out"
            >
              <span className="material-symbols-outlined text-[17px] text-rose-400/90">logout</span>
              <span className="hidden sm:inline">Sign Out</span>
            </button>
          </div>
        </header>
        <main className="relative pt-28 bg-surface min-h-screen w-full px-gutter">
          <div className="flex flex-col w-full pb-32">
            {activeTab === 'home' ? (
              <>
            {/*  Top Greeting & Live Stream Status  */}
            <section className="flex flex-col xl:flex-row xl:items-end justify-between gap-space-lg mb-space-xl">
              <div className="space-y-space-xs">
                <div className="flex items-center gap-space-xs">
                  <span className="font-label-mono-sm text-label-mono-sm uppercase text-primary tracking-widest font-semibold">
                    TgDrive :: {botPool.online > 0 ? `${botPool.online} Shard Bots Online` : 'Encrypted Storage Node'}
                  </span>
                  <span className="w-1.5 h-1.5 rounded-full bg-primary animate-ping"></span>
                </div>
                <h1 className="font-headline-lg text-headline-lg text-on-surface tracking-tight font-bold">
                  {greeting}
                </h1>
                <p className="font-body-lg text-body-lg text-on-surface-variant">
                  {activeTransfersCount > 0
                    ? `${activeTransfersCount} transfer${activeTransfersCount > 1 ? 's' : ''} running in the background.`
                    : "Your encrypted cloud drive is up to date."}
                </p>
              </div>
            </section>

            {/* Quick Library Categories */}
            <section className="mb-space-xl">
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-space-md">
                <div 
                  onClick={() => setActiveTab('cinema-and-stream')}
                  className="flex items-center gap-space-md p-space-md rounded-2xl bg-surface-container-low/80 backdrop-blur border border-outline-variant/30 hover:bg-surface-container hover:border-primary/40 hover:shadow-lg transition-all cursor-pointer group"
                >
                  <div className="w-12 h-12 rounded-xl bg-primary-container/20 text-primary flex items-center justify-center group-hover:scale-105 transition-transform border border-primary/20">
                    <span className="material-symbols-outlined text-[24px]">theaters</span>
                  </div>
                  <div className="flex flex-col min-w-0">
                    <span className="font-headline-sm text-sm font-bold text-on-surface group-hover:text-primary transition-colors">Cinema &amp; Stream</span>
                    <span className="font-body-sm text-xs text-on-surface-variant truncate">
                      {cinemaVideos.length} in Cinema · {allVideoFiles.length} videos
                    </span>
                  </div>
                </div>

                <div 
                  onClick={() => setActiveTab('music-and-audio')}
                  className="flex items-center gap-space-md p-space-md rounded-2xl bg-surface-container-low/80 backdrop-blur border border-outline-variant/30 hover:bg-surface-container hover:border-primary/40 hover:shadow-lg transition-all cursor-pointer group"
                >
                  <div className="w-12 h-12 rounded-xl bg-surface-container-high text-primary flex items-center justify-center group-hover:scale-105 transition-transform border border-outline-variant/40">
                    <span className="material-symbols-outlined text-[24px]">headphones</span>
                  </div>
                  <div className="flex flex-col min-w-0">
                    <span className="font-headline-sm text-sm font-bold text-on-surface group-hover:text-primary transition-colors">Music &amp; Audio</span>
                    <span className="font-body-sm text-xs text-on-surface-variant truncate">
                      {files.filter(f => getPreviewType(f.mime_type, f.name) === 'audio').length} tracks · {audioGB} GB
                    </span>
                  </div>
                </div>

                <div 
                  onClick={() => setActiveTab('photos')}
                  className="flex items-center gap-space-md p-space-md rounded-2xl bg-surface-container-low/80 backdrop-blur border border-outline-variant/30 hover:bg-surface-container hover:border-primary/40 hover:shadow-lg transition-all cursor-pointer group"
                >
                  <div className="w-12 h-12 rounded-xl bg-surface-container-high text-primary flex items-center justify-center group-hover:scale-105 transition-transform border border-outline-variant/40">
                    <span className="material-symbols-outlined text-[24px]">photo_library</span>
                  </div>
                  <div className="flex flex-col min-w-0">
                    <span className="font-headline-sm text-sm font-bold text-on-surface group-hover:text-primary transition-colors">Photos &amp; Media</span>
                    <span className="font-body-sm text-xs text-on-surface-variant truncate">
                      {files.filter(f => f.mime_type.startsWith('image/')).length} photos · {photosGB} GB
                    </span>
                  </div>
                </div>

                <div 
                  onClick={() => setActiveTab('documents')}
                  className="flex items-center gap-space-md p-space-md rounded-2xl bg-surface-container-low/80 backdrop-blur border border-outline-variant/30 hover:bg-surface-container hover:border-primary/40 hover:shadow-lg transition-all cursor-pointer group"
                >
                  <div className="w-12 h-12 rounded-xl bg-surface-container-high text-primary flex items-center justify-center group-hover:scale-105 transition-transform border border-outline-variant/40">
                    <span className="material-symbols-outlined text-[24px]">description</span>
                  </div>
                  <div className="flex flex-col min-w-0">
                    <span className="font-headline-sm text-sm font-bold text-on-surface group-hover:text-primary transition-colors">Documents &amp; Vault</span>
                    <span className="font-body-sm text-xs text-on-surface-variant truncate">
                      {files.filter(f => f.mime_type.includes('pdf') || f.mime_type.includes('document') || ['.pdf', '.doc', '.docx', '.txt', '.md'].some(ext => f.name.toLowerCase().endsWith(ext))).length} docs · {docsGB} GB
                    </span>
                  </div>
                </div>
              </div>
            </section>

            {/*  Section 3: Lower Split Layout (Recently Opened & Storage Overview)  */}
            <section className="grid grid-cols-1 lg:grid-cols-12 gap-space-lg">
              {/*  Left: Recently Opened List (7 Cols)  */}
              <div className="lg:col-span-7 flex flex-col rounded-2xl bg-surface-container-low/80 backdrop-blur-md p-space-lg shadow-md border border-outline-variant/40">
                <div className="flex items-center justify-between mb-space-md">
                  <div className="flex items-center gap-space-sm">
                    <div className="w-7 h-7 rounded-lg bg-primary-container/20 flex items-center justify-center text-primary">
                      <span className="material-symbols-outlined text-[18px]">history</span>
                    </div>
                    <h2 className="font-headline-sm text-headline-sm text-on-surface font-bold">
                      Recently Opened
                    </h2>
                  </div>
                  <span className="font-label-mono-sm text-label-mono-sm text-on-surface-variant">
                    Updated real-time
                  </span>
                </div>
                <div className="space-y-1 flex-1">
                  {files.slice(0, 6).map((f) => (
                    <div
                      key={f.id}
                      onClick={() => handlePreview(f)}
                      className="flex items-center justify-between p-space-sm rounded-xl hover:bg-surface-container-high/60 transition-all group cursor-pointer"
                    >
                      <div className="flex items-center gap-space-sm min-w-0">
                        <div className="w-9 h-9 rounded-lg bg-surface-container-highest text-primary flex items-center justify-center flex-shrink-0 border border-outline-variant/40 group-hover:scale-105 transition-transform">
                          <span className="material-symbols-outlined text-[20px]">{getFileIcon(f)}</span>
                        </div>
                        <div className="flex flex-col min-w-0">
                          <span className="font-body-md text-body-md text-on-surface font-medium truncate group-hover:text-primary transition-colors">
                            {f.name}
                          </span>
                          <span className="font-body-sm text-body-sm text-on-surface-variant truncate">
                            {getFileCategory(f.mime_type, f.name)} · {formatDate(f.created_at)}
                          </span>
                        </div>
                      </div>
                      <div className="flex items-center gap-space-md flex-shrink-0">
                        <span className="font-label-mono-sm text-label-mono-sm text-on-surface-variant">
                          {formatBytes(f.size)}
                        </span>
                        <button
                          aria-label={`${f.name} options`}
                          className="opacity-0 group-hover:opacity-100 text-outline hover:text-on-surface hover:bg-surface-container-highest rounded-lg transition-all p-1.5"
                          type="button"
                          onClick={(e) => handleOpenMenu(e, f)}
                        >
                          <span className="material-symbols-outlined text-[18px]">more_vert</span>
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
              {/*  Right: Storage Overview Card (5 Cols)  */}
              <div className="lg:col-span-5 flex flex-col justify-between rounded-2xl bg-surface-container-low/80 backdrop-blur-md p-space-lg shadow-md border border-outline-variant/40">
                <div>
                  <div className="flex items-center justify-between mb-space-sm">
                    <div className="flex items-center gap-space-sm">
                      <div className="w-7 h-7 rounded-lg bg-surface-bright flex items-center justify-center text-primary">
                        <span className="material-symbols-outlined text-[18px]">pie_chart</span>
                      </div>
                      <h2 className="font-headline-sm text-headline-sm text-on-surface font-bold">
                        Storage Overview
                      </h2>
                    </div>
                  </div>
                  <div className="my-space-md">
                    <div className="flex items-baseline gap-space-xs">
                      <span className="font-display-hero text-display-hero text-on-surface font-bold tracking-tight">
                        {totalGB}
                      </span>
                      <span className="font-headline-md text-headline-md text-on-surface-variant font-medium ml-1">
                        GB used
                      </span>
                      <span className="font-body-sm text-body-sm text-outline ml-2 mr-1">of</span>
                      <span className="font-body-sm text-body-sm text-primary opacity-80 whitespace-nowrap" title="TgDrive offers unlimited storage space">
                        <span className="material-symbols-outlined text-[16px] align-text-bottom mr-1">all_inclusive</span>
                        unlimited
                      </span>
                    </div>
                    {/* Smooth Multi-color Segmented Storage Bar */}
                    <div 
                      className="w-full h-3 rounded-full bg-surface-container-highest/70 overflow-hidden flex mt-space-sm cursor-pointer shadow-inner border border-outline-variant/20 relative"
                      onClick={() => setShowSettingsModal(true)}
                    >
                      {totalBytes === 0 ? (
                        <div className="w-full h-full bg-surface-container-highest/40" />
                      ) : (
                        <>
                          <div
                            className="bg-[#d99b4e] h-full transition-[width] duration-700 ease-out hover:brightness-110"
                            style={{ width: videoPct }}
                            title={`Video: ${videoGB} GB (${videoPct})`}
                          />
                          <div
                            className="bg-[#e5b066] h-full transition-[width] duration-700 ease-out hover:brightness-110"
                            style={{ width: photosPct }}
                            title={`Photos: ${photosGB} GB (${photosPct})`}
                          />
                          <div
                            className="bg-[#c2883e] h-full transition-[width] duration-700 ease-out hover:brightness-110"
                            style={{ width: audioPct }}
                            title={`Audio: ${audioGB} GB (${audioPct})`}
                          />
                          <div
                            className="bg-[#a89078] h-full transition-[width] duration-700 ease-out hover:brightness-110"
                            style={{ width: archivesPct }}
                            title={`Archives: ${archivesGB} GB (${archivesPct})`}
                          />
                          <div
                            className="bg-[#786c5f] h-full transition-[width] duration-700 ease-out hover:brightness-110"
                            style={{ width: docsPct }}
                            title={`Documents: ${docsGB} GB (${docsPct})`}
                          />
                          <div
                            className="bg-[#524940] h-full transition-[width] duration-700 ease-out hover:brightness-110"
                            style={{ width: othersPct }}
                            title={`Others: ${othersGB} GB (${othersPct})`}
                          />
                        </>
                      )}
                    </div>
                  </div>
                  {/*  Breakdown Grid  */}
                  <div className="grid grid-cols-2 gap-x-space-md gap-y-space-sm my-space-md">
                    <div onClick={() => setActiveTab('videos')} className="flex items-center justify-between p-2 rounded-xl bg-surface-container/70 border border-outline-variant/40 hover:bg-surface-container-high transition-colors cursor-pointer group">
                      <div className="flex items-center gap-2">
                        <span className="w-2.5 h-2.5 rounded-full bg-[#d99b4e]"></span>
                        <span className="font-body-sm text-body-sm text-on-surface group-hover:text-primary transition-colors">
                          Video
                        </span>
                      </div>
                      <span className="font-label-mono-sm text-label-mono-sm text-on-surface-variant">
                        {videoGB} GB
                      </span>
                    </div>
                    <div onClick={() => setActiveTab('photos')} className="flex items-center justify-between p-2 rounded-xl bg-surface-container/70 border border-outline-variant/40 hover:bg-surface-container-high transition-colors cursor-pointer group">
                      <div className="flex items-center gap-2">
                        <span className="w-2.5 h-2.5 rounded-full bg-[#e5b066]"></span>
                        <span className="font-body-sm text-body-sm text-on-surface group-hover:text-primary transition-colors">
                          Photos
                        </span>
                      </div>
                      <span className="font-label-mono-sm text-label-mono-sm text-on-surface-variant">
                        {photosGB} GB
                      </span>
                    </div>
                    <div onClick={() => setActiveTab('audio')} className="flex items-center justify-between p-2 rounded-xl bg-surface-container/70 border border-outline-variant/40 hover:bg-surface-container-high transition-colors cursor-pointer group">
                      <div className="flex items-center gap-2">
                        <span className="w-2.5 h-2.5 rounded-full bg-[#c2883e]"></span>
                        <span className="font-body-sm text-body-sm text-on-surface group-hover:text-primary transition-colors">
                          Audio
                        </span>
                      </div>
                      <span className="font-label-mono-sm text-label-mono-sm text-on-surface-variant">
                        {audioGB} GB
                      </span>
                    </div>
                    <div onClick={() => setActiveTab('archives')} className="flex items-center justify-between p-2 rounded-xl bg-surface-container/70 border border-outline-variant/40 hover:bg-surface-container-high transition-colors cursor-pointer group">
                      <div className="flex items-center gap-2">
                        <span className="w-2.5 h-2.5 rounded-full bg-[#a89078]"></span>
                        <span className="font-body-sm text-body-sm text-on-surface group-hover:text-primary transition-colors">
                          Archives
                        </span>
                      </div>
                      <span className="font-label-mono-sm text-label-mono-sm text-on-surface-variant">
                        {archivesGB} GB
                      </span>
                    </div>
                    <div onClick={() => setActiveTab('documents')} className="flex items-center justify-between p-2 rounded-xl bg-surface-container/70 border border-outline-variant/40 hover:bg-surface-container-high transition-colors cursor-pointer group">
                      <div className="flex items-center gap-2">
                        <span className="w-2.5 h-2.5 rounded-full bg-[#786c5f]"></span>
                        <span className="font-body-sm text-body-sm text-on-surface group-hover:text-primary transition-colors">
                          Documents
                        </span>
                      </div>
                      <span className="font-label-mono-sm text-label-mono-sm text-on-surface-variant">
                        {docsGB} GB
                      </span>
                    </div>
                    <div onClick={() => setActiveTab('others')} className="flex items-center justify-between p-2 rounded-xl bg-surface-container/70 border border-outline-variant/40 hover:bg-surface-container-high transition-colors cursor-pointer group">
                      <div className="flex items-center gap-2">
                        <span className="w-2.5 h-2.5 rounded-full bg-[#524940]"></span>
                        <span className="font-body-sm text-body-sm text-on-surface group-hover:text-primary transition-colors">
                          Others
                        </span>
                      </div>
                      <span className="font-label-mono-sm text-label-mono-sm text-on-surface-variant">
                        {othersGB} GB
                      </span>
                    </div>
                  </div>
                </div>
                <div className="pt-space-sm border-t border-outline-variant/30 flex items-center justify-between font-label-mono-sm text-label-mono-sm text-on-surface-variant">
                  <span onClick={() => setActiveTab('all-files')} className="cursor-pointer hover:text-primary transition-colors">{totalFilesStr} files</span>
                  <span onClick={() => setShowSettingsModal(true)} className="flex items-center gap-1 text-primary cursor-pointer hover:underline" title="Inspect AES-256-GCM Cluster">
                    <span className="material-symbols-outlined text-[14px]">lock</span>
                    AES-256
                  </span>
                </div>
              </div>
            </section>
            </>
            ) : activeTab === 'cinema-and-stream' ? (
              <div className="flex-1 flex flex-col w-full px-space-md lg:px-8 py-5 pb-32 max-w-7xl mx-auto overflow-y-auto">
                {/* Cinema Tab Header & Management Controls */}
                <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
                  <div className="flex items-center gap-3">
                    <div className="w-12 h-12 rounded-2xl bg-surface-container-high border border-outline-variant/30 flex items-center justify-center text-primary shadow-sm">
                      <span className="material-symbols-outlined text-[26px]">theaters</span>
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <h1 className="text-xl md:text-2xl font-bold text-on-surface">Cinema &amp; Stream</h1>
                        <span className="px-2 py-0.5 rounded-full bg-primary/15 text-primary border border-primary/25 text-xs font-mono font-semibold">
                          {cinemaVideos.length} {cinemaVideos.length === 1 ? 'title' : 'titles'}
                        </span>
                      </div>
                      <p className="text-xs text-on-surface-variant mt-0.5">Your private streaming room • Curated videos &amp; films</p>
                    </div>
                  </div>

                  <div className="flex items-center gap-2.5">
                    {/* Live search in Cinema */}
                    {cinemaVideos.length > 0 && (
                      <div className="relative flex items-center">
                        <span className="material-symbols-outlined absolute left-3 text-on-surface-variant text-[18px] pointer-events-none">search</span>
                        <input 
                          type="text" 
                          value={cinemaSearchQuery}
                          onChange={(e) => setCinemaSearchQuery(e.target.value)}
                          placeholder="Search cinema..." 
                          className="pl-9 pr-8 py-2 rounded-xl bg-surface-container border border-outline-variant/30 text-xs text-on-surface placeholder:text-on-surface-variant/60 focus:outline-none focus:border-primary/60 w-36 sm:w-52 transition-all shadow-inner"
                        />
                        {cinemaSearchQuery && (
                          <button 
                            onClick={() => setCinemaSearchQuery('')}
                            className="absolute right-2.5 text-on-surface-variant hover:text-on-surface p-0.5 rounded-full cursor-pointer"
                            title="Clear search"
                          >
                            <span className="material-symbols-outlined text-[14px]">close</span>
                          </button>
                        )}
                      </div>
                    )}

                    {/* View mode toggle: Shelves vs Grid */}
                    {cinemaVideos.length > 0 && (
                      <div className="flex items-center bg-surface-container rounded-xl p-0.5 border border-outline-variant/30">
                        <button
                          onClick={() => setCinemaViewMode('shelves')}
                          className={`p-1.5 rounded-lg transition-colors cursor-pointer ${cinemaViewMode === 'shelves' ? 'bg-surface-container-high text-primary shadow-sm' : 'text-on-surface-variant hover:text-on-surface'}`}
                          title="Carousel Shelves View"
                        >
                          <span className="material-symbols-outlined text-[18px]">view_timeline</span>
                        </button>
                        <button
                          onClick={() => setCinemaViewMode('grid')}
                          className={`p-1.5 rounded-lg transition-colors cursor-pointer ${cinemaViewMode === 'grid' ? 'bg-surface-container-high text-primary shadow-sm' : 'text-on-surface-variant hover:text-on-surface'}`}
                          title="Poster Grid View"
                        >
                          <span className="material-symbols-outlined text-[18px]">grid_view</span>
                        </button>
                      </div>
                    )}

                    {/* Quick Manage Titles Button */}
                    <button
                      onClick={() => setShowAddCinemaModal(true)}
                      className="flex items-center gap-1.5 bg-primary/10 text-primary hover:bg-primary/20 border border-primary/30 px-3 py-2 rounded-xl text-xs font-semibold transition-colors cursor-pointer"
                      title="Manage Cinema Collection"
                    >
                      <span className="material-symbols-outlined text-[16px]">movie_edit</span>
                      <span>Manage Titles</span>
                    </button>
                  </div>
                </div>

                {/* Hero Showcase Panel - Cinema Widescreen Aspect Ratio (320px - 400px height) */}
                <div className="relative w-full h-[280px] sm:h-[340px] md:h-[380px] lg:h-[400px] max-h-[420px] rounded-2xl md:rounded-3xl overflow-hidden mb-8 border border-outline-variant/25 shadow-2xl bg-surface-container-lowest group">
                  {heroMovie ? (
                    <>
                      <VideoThumbnail 
                        file={heroMovie} 
                        isHero={true} 
                        className="absolute inset-0 w-full h-full object-cover object-center z-0 group-hover:scale-[1.02] transition-transform duration-1000 ease-out" 
                      />
                      {/* Cinema Panoramic Dual Gradient Masks */}
                      <div className="absolute inset-0 bg-gradient-to-t from-[#0e0f12] via-[#0e0f12]/50 to-transparent z-10" />
                      <div className="absolute inset-0 bg-gradient-to-r from-[#0e0f12] via-[#0e0f12]/80 to-transparent z-10" />

                      {/* Content Area */}
                      <div className="absolute bottom-0 left-0 p-6 sm:p-8 z-20 w-full max-w-2xl">
                        <div className="flex flex-wrap items-center gap-2 mb-2.5">
                          <span className="inline-block px-2.5 py-0.5 bg-primary/20 text-primary border border-primary/30 rounded-md text-[11px] font-bold tracking-widest uppercase">
                            CINEMA FEATURED
                          </span>
                          <span className="px-2 py-0.5 rounded-md bg-black/60 backdrop-blur-md text-white/90 border border-white/20 text-[11px] font-mono font-semibold">
                            {heroMovie.size > 8 * 1024 * 1024 * 1024 ? '4K UHD' : heroMovie.size > 1.5 * 1024 * 1024 * 1024 ? '1080p' : 'HD'}
                          </span>
                          <span className="px-2 py-0.5 rounded-md bg-black/60 backdrop-blur-md text-primary border border-primary/30 text-[11px] font-mono font-bold">
                            {formatFileSize(heroMovie.size)}
                          </span>
                          {heroMovie.metadata?.duration && (
                            <span className="px-2 py-0.5 rounded-md bg-black/60 backdrop-blur-md text-white/80 border border-white/15 text-[11px] font-mono">
                              {Math.floor(heroMovie.metadata.duration / 60)}m
                            </span>
                          )}
                        </div>

                        <h2 className="text-2xl sm:text-3xl md:text-4xl font-black tracking-tight text-white mb-2 line-clamp-1 drop-shadow-md">
                          {getCleanMovieTitle(heroMovie.name)}
                        </h2>

                        <p className="text-xs sm:text-sm text-white/70 mb-4 line-clamp-1 font-sans">
                          {heroMovie.name} • Added {formatDate(heroMovie.created_at)}
                        </p>

                        <div className="flex flex-wrap items-center gap-2.5">
                          <button 
                            onClick={() => handlePreview(heroMovie)}
                            className="flex items-center gap-2 bg-primary text-on-primary-fixed px-6 py-2.5 rounded-xl text-sm font-bold hover:bg-primary-fixed transition-colors shadow-[0_0_20px_rgba(252,186,105,0.4)] cursor-pointer"
                          >
                            <span className="material-symbols-outlined filled text-[20px]">play_arrow</span>
                            Play Movie
                          </button>
                          <button 
                            onClick={() => {
                              setSelectedMovieForInfo(heroMovie);
                              setShowMovieInfoModal(true);
                            }}
                            className="flex items-center gap-1.5 bg-white/10 hover:bg-white/20 text-white backdrop-blur px-4 py-2.5 rounded-xl font-semibold text-xs sm:text-sm transition-colors border border-white/20 cursor-pointer"
                          >
                            <span className="material-symbols-outlined text-[18px]">info</span>
                            More Info
                          </button>
                          <button 
                            onClick={() => handleToggleCinema(heroMovie)}
                            className="flex items-center gap-1.5 bg-white/10 hover:bg-rose-500/20 text-rose-400 backdrop-blur px-3.5 py-2.5 rounded-xl font-semibold text-xs sm:text-sm transition-colors border border-white/15 cursor-pointer"
                            title="Remove from Cinema"
                          >
                            <span className="material-symbols-outlined text-[16px]">movie_edit</span>
                            Remove
                          </button>
                        </div>
                      </div>

                      {/* Interactive Carousel Pill for Multiple Cinema Movies */}
                      {cinemaVideos.length > 1 && (
                        <div className="absolute bottom-6 right-6 z-20 flex items-center gap-2 bg-black/60 backdrop-blur-md px-3 py-1.5 rounded-full border border-white/15 shadow-lg">
                          <span className="text-[10px] font-mono text-white/70">
                            {cinemaVideos.findIndex(f => f.id === heroMovie.id) + 1} / {cinemaVideos.length}
                          </span>
                          <div className="flex items-center gap-1">
                            {cinemaVideos.slice(0, 6).map((f) => (
                              <button
                                key={f.id}
                                onClick={() => setSelectedHeroId(f.id)}
                                className={`h-2 rounded-full transition-all cursor-pointer ${f.id === heroMovie.id ? 'w-5 bg-primary' : 'w-2 bg-white/35 hover:bg-white/70'}`}
                                title={getCleanMovieTitle(f.name)}
                              />
                            ))}
                          </div>
                        </div>
                      )}
                    </>
                  ) : (
                    <div className="absolute inset-0 flex flex-col items-center justify-center text-center p-6 sm:p-8 z-10 bg-surface-container-low/60 backdrop-blur-sm">
                      <div className="w-14 h-14 rounded-2xl bg-surface-container-high border border-outline-variant/30 flex items-center justify-center text-primary mb-3 shadow-md">
                        <span className="material-symbols-outlined text-3xl">theaters</span>
                      </div>
                      <h3 className="text-lg md:text-xl font-bold text-on-surface mb-1.5">Your Cinema Showcase is Empty</h3>
                      <p className="text-xs text-on-surface-variant max-w-md mb-5 leading-relaxed">
                        Videos are not added automatically. To showcase movies here, click <strong>Manage Titles</strong> or use the <strong>3-dots (⋮)</strong> menu on any video in your drive.
                      </p>
                      <div className="flex flex-wrap items-center justify-center gap-3">
                        <button 
                          onClick={() => setShowAddCinemaModal(true)}
                          className="flex items-center gap-2 bg-primary text-on-primary-fixed px-5 py-2.5 rounded-xl font-bold text-xs hover:bg-primary-fixed transition-colors shadow-md cursor-pointer"
                        >
                          <span className="material-symbols-outlined text-[16px]">add_to_photos</span>
                          Manage Cinema Titles ({allVideoFiles.length} videos)
                        </button>
                        <button 
                          onClick={() => setActiveTab('videos')}
                          className="flex items-center gap-1.5 bg-surface-container-high hover:bg-surface-bright text-on-surface px-4 py-2.5 rounded-xl font-semibold text-xs transition-colors border border-outline-variant/30 cursor-pointer"
                        >
                          <span className="material-symbols-outlined text-[16px]">video_library</span>
                          Browse Video Library
                        </button>
                      </div>
                    </div>
                  )}
                </div>

                {/* If searching within Cinema and no matches */}
                {cinemaSearchQuery && filteredCinemaVideos.length === 0 && (
                  <div className="py-16 text-center text-on-surface-variant text-sm">
                    <span className="material-symbols-outlined text-4xl mb-2 text-outline">search_off</span>
                    <p className="font-semibold text-on-surface">No titles found</p>
                    <p className="text-xs text-outline mt-1">No cinema videos match "{cinemaSearchQuery}".</p>
                  </div>
                )}

                {/* View Mode: Carousel Shelves */}
                {cinemaViewMode === 'shelves' ? (
                  <>
                    {/* Row 1: Continue Watching (only if in progress) */}
                    {filteredCinemaVideos.filter(f => (f.progress || 0) > 0).length > 0 && (
                      <div className="mb-8 px-0.5">
                        <div className="flex items-center justify-between mb-3">
                          <h3 className="text-base font-bold text-on-surface flex items-center gap-2">
                            <span>Continue Watching</span>
                            <span className="w-1.5 h-1.5 rounded-full bg-primary"></span>
                          </h3>
                          <span className="text-xs text-on-surface-variant font-mono">
                            {filteredCinemaVideos.filter(f => (f.progress || 0) > 0).length} in progress
                          </span>
                        </div>
                        <div className="flex gap-4 overflow-x-auto pb-3 custom-scrollbar">
                          {filteredCinemaVideos.filter(f => (f.progress || 0) > 0).map((vFile, idx) => (
                            <NetflixVideoCard
                              key={`cw-${vFile.id}-${idx}`}
                              file={vFile}
                              onPlay={handlePreview}
                              onOpenInfo={(f) => {
                                setSelectedMovieForInfo(f);
                                setShowMovieInfoModal(true);
                              }}
                              onAddToCategory={() => setShowCreateCategoryModal(true)}
                              onToggleFavorite={handleToggleFavorite}
                              isFavorite={favorites.has(vFile.id)}
                              onOpenMenu={handleOpenMenu}
                              progressPercent={vFile.progress ? Math.round(vFile.progress * 100) : 0}
                            />
                          ))}
                        </div>
                      </div>
                    )}

                    {/* Row 2: All Cinema Titles */}
                    {filteredCinemaVideos.length > 0 && (
                      <div className="mb-8 px-0.5">
                        <div className="flex items-center justify-between mb-3">
                          <div className="flex items-center gap-2">
                            <h3 className="text-base font-bold text-on-surface">Cinema Collection</h3>
                            <span className="text-xs text-on-surface-variant font-mono">({filteredCinemaVideos.length})</span>
                          </div>
                          <span className="text-xs text-on-surface-variant">Hover to preview &amp; stream</span>
                        </div>
                        <div className="flex gap-4 overflow-x-auto pb-3 custom-scrollbar">
                          {filteredCinemaVideos.map((vFile, idx) => (
                            <NetflixVideoCard
                              key={`all-${vFile.id}-${idx}`}
                              file={vFile}
                              onPlay={handlePreview}
                              onOpenInfo={(f) => {
                                setSelectedMovieForInfo(f);
                                setShowMovieInfoModal(true);
                              }}
                              onAddToCategory={() => setShowCreateCategoryModal(true)}
                              onToggleFavorite={handleToggleFavorite}
                              isFavorite={favorites.has(vFile.id)}
                              onOpenMenu={handleOpenMenu}
                              progressPercent={vFile.progress ? Math.round(vFile.progress * 100) : 0}
                            />
                          ))}
                        </div>
                      </div>
                    )}
                  </>
                ) : (
                  /* View Mode: Grid View */
                  filteredCinemaVideos.length > 0 && (
                    <div className="mb-8 px-0.5">
                      <div className="flex items-center justify-between mb-4">
                        <h3 className="text-base font-bold text-on-surface">Cinema Gallery</h3>
                        <span className="text-xs text-on-surface-variant font-mono">{filteredCinemaVideos.length} titles</span>
                      </div>
                      <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
                        {filteredCinemaVideos.map((vFile, idx) => (
                          <div key={`grid-${vFile.id}-${idx}`} className="w-full">
                            <NetflixVideoCard
                              file={vFile}
                              className="w-full"
                              onPlay={handlePreview}
                              onOpenInfo={(f) => {
                                setSelectedMovieForInfo(f);
                                setShowMovieInfoModal(true);
                              }}
                              onAddToCategory={() => setShowCreateCategoryModal(true)}
                              onToggleFavorite={handleToggleFavorite}
                              isFavorite={favorites.has(vFile.id)}
                              onOpenMenu={handleOpenMenu}
                              progressPercent={vFile.progress ? Math.round(vFile.progress * 100) : 0}
                            />
                          </div>
                        ))}
                      </div>
                    </div>
                  )
                )}
              </div>
            ) : activeTab === 'music-and-audio' ? (
              <div className="flex-1 flex flex-col w-full px-space-md lg:px-8 py-6 pb-32 max-w-5xl mx-auto overflow-y-auto">
                {/* Header: Title, Description, and Live Search */}
                <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
                  <div className="flex items-center gap-3">
                    <div className="w-12 h-12 rounded-xl bg-surface-container-high border border-outline-variant/30 flex items-center justify-center text-primary shadow-sm">
                      <span className="material-symbols-outlined text-[26px]">headphones</span>
                    </div>
                    <div>
                      <h1 className="text-xl font-bold text-on-surface">TgDrive Music</h1>
                      <p className="text-body-sm text-on-surface-variant">Your music. Your library. No clutter.</p>
                    </div>
                  </div>

                  {/* Live Search Input */}
                  <div className="relative flex items-center">
                    <span className="material-symbols-outlined absolute left-3 text-on-surface-variant text-[18px] pointer-events-none">search</span>
                    <input 
                      type="text" 
                      value={musicSearchQuery}
                      onChange={(e) => setMusicSearchQuery(e.target.value)}
                      placeholder="Search songs, artists, formats..." 
                      className="pl-9 pr-8 py-2 rounded-xl bg-surface-container border border-outline-variant/30 text-sm text-on-surface placeholder:text-on-surface-variant/60 focus:outline-none focus:border-primary/60 w-full sm:w-72 transition-all shadow-inner"
                    />
                    {musicSearchQuery && (
                      <button 
                        onClick={() => setMusicSearchQuery('')}
                        className="absolute right-2.5 text-on-surface-variant hover:text-on-surface p-1 rounded-full cursor-pointer"
                        title="Clear search"
                      >
                        <span className="material-symbols-outlined text-[16px]">close</span>
                      </button>
                    )}
                  </div>
                </div>

                {/* Greeting & 4 Interactive Quick Filter Tiles */}
                <div className="mb-8">
                  <h2 className="text-lg font-bold text-on-surface mb-1">{greeting}</h2>
                  <p className="text-body-sm text-on-surface-variant mb-4">Pick up where you left off or browse collections.</p>
                  
                  <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 gap-3">
                    {[
                      { 
                        id: 'recent' as const, 
                        icon: 'schedule', 
                        title: 'Recently played', 
                        desc: `${recentAudioFiles.length} ${recentAudioFiles.length === 1 ? 'track' : 'tracks'}` 
                      },
                      { 
                        id: 'queue' as const, 
                        icon: 'queue_music', 
                        title: 'Play Queue', 
                        desc: `${playlist.length} queued` 
                      },
                      { 
                        id: 'folders' as const, 
                        icon: 'folder', 
                        title: 'Folders', 
                        desc: `${audioFolders.length} ${audioFolders.length === 1 ? 'folder' : 'folders'}` 
                      },
                      { 
                        id: 'favorites' as const, 
                        icon: 'favorite', 
                        title: 'Favorites', 
                        desc: `${favoriteAudioFiles.length} ${favoriteAudioFiles.length === 1 ? 'track' : 'tracks'}` 
                      }
                    ].map((tile) => {
                      const isActive = musicFilter === tile.id;
                      return (
                        <div 
                          key={tile.id} 
                          onClick={() => {
                            if (musicFilter === tile.id) {
                              setMusicFilter('all');
                              setSelectedMusicFolderId(null);
                            } else {
                              setMusicFilter(tile.id);
                              if (tile.id !== 'folders') setSelectedMusicFolderId(null);
                            }
                          }}
                          className={`flex items-start gap-3.5 p-3.5 rounded-xl border transition-all cursor-pointer select-none ${
                            isActive 
                              ? 'bg-primary/10 border-primary shadow-[0_0_15px_rgba(252,186,105,0.15)] ring-1 ring-primary/40' 
                              : 'bg-surface-container-low hover:bg-surface-container border-outline-variant/20 hover:border-outline-variant/40'
                          }`}
                        >
                          <div className={`p-2 rounded-lg flex items-center justify-center ${isActive ? 'bg-primary text-on-primary' : 'bg-surface-container text-on-surface-variant'}`}>
                            <span className="material-symbols-outlined text-[20px]">{tile.icon}</span>
                          </div>
                          <div className="min-w-0 flex-1">
                            <h3 className={`font-semibold text-sm truncate ${isActive ? 'text-primary' : 'text-on-surface'}`}>{tile.title}</h3>
                            <p className="text-xs text-on-surface-variant mt-0.5 truncate">{tile.desc}</p>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </div>

                {/* Active Filter Pill & Reset */}
                {(musicFilter !== 'all' || musicSearchQuery || selectedMusicFolderId) && (
                  <div className="flex items-center justify-between mb-6 px-4 py-2.5 rounded-xl bg-surface-container border border-outline-variant/30 text-sm">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="text-on-surface-variant text-xs">Active view:</span>
                      <span className="px-2.5 py-0.5 rounded-full bg-primary/20 text-primary border border-primary/30 text-xs font-semibold">
                        {musicFilter === 'recent' 
                          ? 'Recently Played' 
                          : musicFilter === 'queue' 
                            ? 'Play Queue' 
                            : musicFilter === 'favorites' 
                              ? 'Favorites' 
                              : musicFilter === 'folders' 
                                ? (selectedMusicFolderId 
                                    ? `Folder: ${audioFolders.find(g => g.id === selectedMusicFolderId)?.name || 'Unknown'}` 
                                    : 'Browsing Folders')
                                : 'All Tracks'}
                      </span>
                      {musicSearchQuery && (
                        <span className="text-xs text-on-surface-variant">
                          Matching: <strong className="text-on-surface">"{musicSearchQuery}"</strong>
                        </span>
                      )}
                    </div>
                    <button 
                      onClick={() => {
                        setMusicFilter('all');
                        setMusicSearchQuery('');
                        setSelectedMusicFolderId(null);
                      }}
                      className="text-xs text-on-surface-variant hover:text-primary transition-colors flex items-center gap-1 cursor-pointer font-medium ml-2"
                    >
                      <span className="material-symbols-outlined text-[16px]">close</span> Show All Tracks
                    </button>
                  </div>
                )}

                {/* Sub-section: Folders Grid if musicFilter === 'folders' and no folder selected */}
                {musicFilter === 'folders' && !selectedMusicFolderId && (
                  <div className="mb-8">
                    <div className="flex items-center justify-between mb-3">
                      <h3 className="text-sm font-bold text-on-surface flex items-center gap-2">
                        <span className="material-symbols-outlined text-primary text-[20px]">folder_open</span>
                        Audio Locations ({audioFolders.length})
                      </h3>
                    </div>
                    <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-3">
                      {audioFolders.map(fGroup => (
                        <div 
                          key={fGroup.id}
                          onClick={() => setSelectedMusicFolderId(fGroup.id)}
                          className="flex items-center justify-between p-3.5 rounded-xl bg-surface-container-low hover:bg-surface-container border border-outline-variant/20 hover:border-outline-variant/40 transition-all cursor-pointer group"
                        >
                          <div className="flex items-center gap-3 min-w-0">
                            <div className="w-10 h-10 rounded-lg bg-surface-container flex items-center justify-center text-primary group-hover:scale-105 transition-transform flex-shrink-0">
                              <span className="material-symbols-outlined text-[20px]">folder</span>
                            </div>
                            <div className="min-w-0">
                              <h4 className="font-semibold text-sm text-on-surface truncate group-hover:text-primary transition-colors">{fGroup.name}</h4>
                              <p className="text-xs text-on-surface-variant">{fGroup.count} {fGroup.count === 1 ? 'audio file' : 'audio files'}</p>
                            </div>
                          </div>
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              setPlaylist(fGroup.files);
                              setCurrentTrackIndex(0);
                              setAudioProgressSec(0);
                              setIsPlayingAudio(true);
                              setIsDockMinimized(false);
                              setHasActiveAudioSession(true);
                              showToast(`Playing ${fGroup.files.length} tracks from "${fGroup.name}"`, "success");
                            }}
                            className="p-1.5 rounded-lg bg-surface-container hover:bg-primary hover:text-on-primary text-on-surface-variant transition-colors cursor-pointer ml-2 flex-shrink-0"
                            title="Play all tracks in folder"
                          >
                            <span className="material-symbols-outlined text-[18px]">play_arrow</span>
                          </button>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Tracks Header */}
                <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-4">
                  <div className="flex items-center gap-3">
                    <h2 className="text-lg font-bold text-on-surface">
                      {musicFilter === 'recent' 
                        ? 'Recently Played' 
                        : musicFilter === 'queue' 
                          ? 'Current Queue' 
                          : musicFilter === 'favorites' 
                            ? 'Favorite Tracks' 
                            : musicFilter === 'folders' && selectedMusicFolderId 
                              ? `Tracks in ${audioFolders.find(g => g.id === selectedMusicFolderId)?.name || 'Folder'}` 
                              : 'All Tracks'}
                      <span className="text-on-surface-variant text-sm font-normal ml-2 font-mono">
                        ({displayedMusicTracks.length})
                      </span>
                    </h2>

                    {/* Play All button */}
                    <button 
                      onClick={() => {
                        if (displayedMusicTracks.length > 0) {
                          setPlaylist(displayedMusicTracks);
                          setCurrentTrackIndex(0);
                          setAudioProgressSec(0);
                          setIsPlayingAudio(true);
                          setIsDockMinimized(false);
                          setHasActiveAudioSession(true);
                          showToast(`Playing all ${displayedMusicTracks.length} tracks`, "success");
                        } else {
                          showToast("No tracks to play in current view", "info");
                        }
                      }}
                      className="flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-primary/10 text-primary hover:bg-primary/20 transition-colors text-sm font-medium cursor-pointer"
                      title="Play All Tracks"
                    >
                      <span className="material-symbols-outlined text-[18px]">play_arrow</span>
                      Play All
                    </button>

                    {/* Shuffle Play button */}
                    <button 
                      onClick={() => {
                        if (displayedMusicTracks.length > 0) {
                          setPlaylist(displayedMusicTracks);
                          setIsShuffle(true);
                          const randomIdx = Math.floor(Math.random() * displayedMusicTracks.length);
                          setCurrentTrackIndex(randomIdx);
                          setAudioProgressSec(0);
                          setIsPlayingAudio(true);
                          setIsDockMinimized(false);
                          setHasActiveAudioSession(true);
                          showToast(`Shuffle playing ${displayedMusicTracks.length} tracks`, "success");
                        } else {
                          showToast("No tracks to play in current view", "info");
                        }
                      }}
                      className="flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-surface-container text-on-surface hover:text-primary transition-colors text-sm font-medium cursor-pointer border border-outline-variant/30"
                      title="Shuffle Play All Tracks"
                    >
                      <span className="material-symbols-outlined text-[16px]">shuffle</span>
                      Shuffle
                    </button>
                  </div>

                  {/* Interactive Sort Dropdown */}
                  <div className="relative">
                    <button 
                      onClick={() => setShowMusicSortMenu(!showMusicSortMenu)}
                      className="flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-xl bg-surface-container border border-outline-variant/30 text-on-surface hover:border-primary/50 transition-colors cursor-pointer"
                    >
                      <span className="text-on-surface-variant text-xs">Sort:</span>
                      <span className="font-semibold text-xs">
                        {musicSortBy === 'name' ? 'Name' : musicSortBy === 'date' ? 'Date' : musicSortBy === 'duration' ? 'Duration' : 'Size'}
                      </span>
                      <span className="material-symbols-outlined text-[16px] text-primary">
                        {musicSortOrder === 'asc' ? 'arrow_upward' : 'arrow_downward'}
                      </span>
                    </button>

                    {showMusicSortMenu && (
                      <div className="absolute right-0 top-full mt-1.5 w-48 rounded-xl bg-surface-container-high/95 backdrop-blur-xl border border-outline-variant/40 shadow-xl py-1.5 z-30">
                        {[
                          { key: 'name', order: 'asc', label: 'Name (A to Z)' },
                          { key: 'name', order: 'desc', label: 'Name (Z to A)' },
                          { key: 'date', order: 'desc', label: 'Date (Newest first)' },
                          { key: 'date', order: 'asc', label: 'Date (Oldest first)' },
                          { key: 'duration', order: 'desc', label: 'Duration (Longest first)' },
                          { key: 'size', order: 'desc', label: 'Size (Largest first)' },
                        ].map(opt => (
                          <button
                            key={`${opt.key}-${opt.order}`}
                            onClick={() => {
                              setMusicSortBy(opt.key as any);
                              setMusicSortOrder(opt.order as any);
                              setShowMusicSortMenu(false);
                            }}
                            className={`w-full text-left px-3 py-2 text-xs flex items-center justify-between cursor-pointer transition-colors ${
                              musicSortBy === opt.key && musicSortOrder === opt.order 
                                ? 'text-primary bg-primary/10 font-bold' 
                                : 'text-on-surface hover:bg-surface-container-highest'
                            }`}
                          >
                            <span>{opt.label}</span>
                            {musicSortBy === opt.key && musicSortOrder === opt.order && (
                              <span className="material-symbols-outlined text-[16px] text-primary">check</span>
                            )}
                          </button>
                        ))}
                      </div>
                    )}
                  </div>
                </div>

                {/* Track List */}
                <div className="flex flex-col gap-1.5">
                  {displayedMusicTracks.length === 0 ? (
                    <div className="py-16 text-center border border-outline-variant/20 rounded-2xl bg-surface-container-low/50">
                      <span className="material-symbols-outlined text-4xl text-outline mb-2">music_off</span>
                      <p className="text-on-surface-variant font-medium">No music files found in this view.</p>
                      <p className="text-xs text-on-surface-variant/70 mt-1">
                        {musicFilter !== 'all' || musicSearchQuery
                          ? 'Try clearing active filters or search terms.'
                          : 'Upload .mp3, .flac, .wav, or .m4a files to your TgDrive.'}
                      </p>
                    </div>
                  ) : (
                    displayedMusicTracks.map((file, i) => {
                      const isCurrent = (currentTrack.fileId === file.id || (playlist[currentTrackIndex]?.id === file.id));
                      const isPlayingThis = isCurrent && isPlayingAudio;
                      const parsed = parseAudioInfo(file.name, file.metadata?.artist, file.metadata?.title);
                      const isFav = favorites.has(file.id);

                      return (
                        <div 
                          key={`${file.id}-${i}`}
                          onClick={() => {
                            if (isCurrent) {
                              setIsPlayingAudio(!isPlayingAudio);
                            } else {
                              setPlaylist(displayedMusicTracks);
                              const targetIndex = displayedMusicTracks.findIndex(f => f.id === file.id);
                              setCurrentTrackIndex(targetIndex >= 0 ? targetIndex : 0);
                              setAudioProgressSec(0);
                              setIsPlayingAudio(true);
                              setIsDockMinimized(false);
                              setHasActiveAudioSession(true);
                            }
                          }}
                          className={`flex items-center justify-between p-3 rounded-xl border transition-all cursor-pointer group ${
                            isCurrent 
                              ? 'bg-primary/10 border-primary/50 shadow-sm' 
                              : 'hover:bg-surface-container-low border-transparent hover:border-outline-variant/20'
                          }`}
                        >
                          <div className="flex items-center gap-3.5 min-w-0 flex-1">
                            {/* Artwork / Equalizer Animated Indicator */}
                            <div className={`w-11 h-11 rounded-xl flex items-center justify-center flex-shrink-0 relative overflow-hidden transition-all shadow-sm ${
                              isCurrent ? 'bg-primary/20 border border-primary/40' : 'bg-surface-container border border-outline-variant/30'
                            }`}>
                              {file.metadata?.thumbnail_telegram_file_id ? (
                                <img
                                  src={`/api/files/${file.id}/thumbnail?token=${getToken()}`}
                                  className="w-full h-full object-cover"
                                  alt={parsed.title}
                                />
                              ) : isPlayingThis ? (
                                <div className="flex items-end gap-0.5 h-4">
                                  <span className="w-1 bg-primary rounded-full animate-bounce h-3"></span>
                                  <span className="w-1 bg-primary rounded-full animate-pulse h-4"></span>
                                  <span className="w-1 bg-primary rounded-full animate-bounce h-2" style={{ animationDelay: '150ms' }}></span>
                                </div>
                              ) : (
                                <span className={`material-symbols-outlined text-[20px] ${isCurrent ? 'text-primary' : 'text-on-surface-variant'}`}>
                                  {isCurrent ? 'headphones' : 'music_note'}
                                </span>
                              )}

                              {/* Hover Play / Pause Button Overlay */}
                              <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 flex items-center justify-center transition-opacity">
                                <span className="material-symbols-outlined text-on-primary text-[20px]">
                                  {isPlayingThis ? 'pause' : 'play_arrow'}
                                </span>
                              </div>
                            </div>

                            {/* Title & Artist */}
                            <div className="flex flex-col min-w-0 flex-1">
                              <div className="flex items-center gap-2">
                                <span className={`font-semibold text-sm truncate ${isCurrent ? 'text-primary' : 'text-on-surface'}`}>
                                  {parsed.title}
                                </span>
                                <span className="font-label-mono-sm text-[10px] px-1.5 py-0.2 rounded bg-surface-container-high text-on-surface-variant uppercase border border-outline-variant/40 flex-shrink-0">
                                  {file.name.split('.').pop() || 'AUDIO'}
                                </span>
                              </div>
                              <span className="text-xs text-on-surface-variant truncate mt-0.5">
                                {parsed.artist}
                              </span>
                            </div>
                          </div>

                          {/* Right Controls: Duration, Favorite, Menu */}
                          <div className="flex items-center gap-3 text-on-surface-variant flex-shrink-0 ml-3">
                            <span className="text-xs font-mono">
                              {file.metadata?.duration 
                                ? formatSec(file.metadata.duration) 
                                : isCurrent && trackDurationSec > 0 
                                  ? formatSec(trackDurationSec) 
                                  : formatBytes(file.size)}
                            </span>

                            {/* Direct Favorite Toggle */}
                            <button
                              onClick={(e) => {
                                e.stopPropagation();
                                handleToggleFavorite(file.id);
                              }}
                              className={`p-1.5 rounded-lg transition-colors cursor-pointer ${
                                isFav ? 'text-rose-400 hover:text-rose-500' : 'text-on-surface-variant hover:text-on-surface opacity-0 group-hover:opacity-100'
                              }`}
                              title={isFav ? "Remove Favorite" : "Add to Favorites"}
                            >
                              <span className="material-symbols-outlined text-[18px]">
                                {isFav ? 'favorite' : 'favorite_border'}
                              </span>
                            </button>

                            {/* Quick remove from queue when in queue view */}
                            {musicFilter === 'queue' && (
                              <button
                                onClick={(e) => {
                                  e.stopPropagation();
                                  handleRemoveFileFromQueue(file.id);
                                }}
                                className="p-1.5 rounded-lg text-on-surface-variant hover:text-rose-400 hover:bg-rose-500/10 transition-colors cursor-pointer"
                                title="Remove from queue"
                              >
                                <span className="material-symbols-outlined text-[18px]">playlist_remove</span>
                              </button>
                            )}

                            {/* Options 3-dots Menu */}
                            <button 
                              onClick={(e) => handleOpenMenu(e, file)}
                              className="p-1.5 rounded-lg hover:bg-surface-container-high transition-colors text-on-surface-variant hover:text-on-surface opacity-0 group-hover:opacity-100 cursor-pointer"
                              title="More Options"
                            >
                              <span className="material-symbols-outlined text-[18px]">more_vert</span>
                            </button>
                          </div>
                        </div>
                      );
                    })
                  )}
                </div>
              </div>
            ) : ['all-files', 'videos', 'audio', 'photos', 'documents', 'archives', 'others'].includes(activeTab) ? (
              <div className="flex-1 flex flex-col w-full mt-4 px-space-md">
                 {/* Header: Title, Search, Sort, View Toggle */}
                 <div className="flex flex-col md:flex-row md:items-center justify-between gap-space-md mb-space-xl">
                    <h1 className="text-3xl font-bold text-on-surface capitalize">{activeTab.replace('-', ' ')} {activeTab === 'all-files' ? '' : 'Library'}</h1>
                    
                    <div className="flex items-center gap-space-md">
                      {/* Search */}
                      <div className="relative">
                        <span className="material-symbols-outlined absolute left-3 top-1/2 -translate-y-1/2 text-outline text-[18px]">search</span>
                        <input 
                          type="text" 
                          value={searchQuery}
                          onChange={(e) => setSearchQuery(e.target.value)}
                          placeholder={`Search ${activeTab.replace('-', ' ')}...`} 
                          className="bg-surface-container-low text-on-surface text-sm rounded-xl pl-10 pr-8 py-2 border border-outline-variant/30 focus:border-primary focus:ring-1 focus:ring-primary outline-none min-w-[250px]" 
                        />
                        {searchQuery && (
                          <button onClick={() => setSearchQuery('')} className="absolute right-2.5 top-1/2 -translate-y-1/2 text-outline hover:text-on-surface">
                            <span className="material-symbols-outlined text-[16px]">close</span>
                          </button>
                        )}
                      </div>
                      
                      {/* Sort Dropdown */}
                      <div className="relative">
                        <button 
                          onClick={() => setShowSortMenu(!showSortMenu)}
                          className="flex items-center gap-2 bg-surface-container-high hover:bg-surface-bright border border-outline-variant/40 px-4 py-2 rounded-xl text-sm font-medium transition-colors text-on-surface cursor-pointer"
                          title="Change sorting criterion"
                        >
                          <span className="material-symbols-outlined text-[18px]">sort</span>
                          Sort by {sortBy === 'date' ? 'Date' : sortBy === 'name' ? 'Name' : sortBy === 'size' ? 'Size' : 'Type'}
                          <span className="material-symbols-outlined text-[16px]">arrow_drop_down</span>
                        </button>

                        {showSortMenu && (
                          <>
                            <div 
                              className="fixed inset-0 z-40"
                              onClick={() => setShowSortMenu(false)}
                            />
                            <div className="absolute right-0 top-full mt-2 w-40 z-50 bg-surface-container-high rounded-xl border border-outline-variant/40 shadow-xl overflow-hidden py-1 animate-in fade-in zoom-in-95 duration-100">
                              <button
                                onClick={() => { setSortBy('name'); setShowSortMenu(false); }}
                                className={`w-full flex items-center justify-between px-4 py-2 text-sm transition-colors hover:bg-surface-bright cursor-pointer ${sortBy === 'name' ? 'text-primary font-medium bg-surface-container-highest' : 'text-on-surface'}`}
                              >
                                Name
                                {sortBy === 'name' && <span className="material-symbols-outlined text-[16px]">check</span>}
                              </button>
                              <button
                                onClick={() => { setSortBy('date'); setShowSortMenu(false); }}
                                className={`w-full flex items-center justify-between px-4 py-2 text-sm transition-colors hover:bg-surface-bright cursor-pointer ${sortBy === 'date' ? 'text-primary font-medium bg-surface-container-highest' : 'text-on-surface'}`}
                              >
                                Date
                                {sortBy === 'date' && <span className="material-symbols-outlined text-[16px]">check</span>}
                              </button>
                              <button
                                onClick={() => { setSortBy('size'); setShowSortMenu(false); }}
                                className={`w-full flex items-center justify-between px-4 py-2 text-sm transition-colors hover:bg-surface-bright cursor-pointer ${sortBy === 'size' ? 'text-primary font-medium bg-surface-container-highest' : 'text-on-surface'}`}
                              >
                                Size
                                {sortBy === 'size' && <span className="material-symbols-outlined text-[16px]">check</span>}
                              </button>
                              <button
                                onClick={() => { setSortBy('type'); setShowSortMenu(false); }}
                                className={`w-full flex items-center justify-between px-4 py-2 text-sm transition-colors hover:bg-surface-bright cursor-pointer ${sortBy === 'type' ? 'text-primary font-medium bg-surface-container-highest' : 'text-on-surface'}`}
                              >
                                Type
                                {sortBy === 'type' && <span className="material-symbols-outlined text-[16px]">check</span>}
                              </button>
                            </div>
                          </>
                        )}
                      </div>
                      <button 
                        onClick={() => setSortOrder(prev => prev === 'desc' ? 'asc' : 'desc')}
                        className="p-2 bg-surface-container-high hover:bg-surface-bright border border-outline-variant/40 rounded-xl text-sm font-medium transition-colors text-on-surface cursor-pointer"
                        title={sortOrder === 'desc' ? 'Descending' : 'Ascending'}
                      >
                        <span className="material-symbols-outlined text-[18px]">swap_vert</span>
                      </button>

                      {/* View Toggle */}
                      <div className="flex items-center bg-surface-container-high rounded-xl border border-outline-variant/40 overflow-hidden">
                         <button onClick={() => setViewMode('grid')} className={`p-2 flex items-center justify-center transition-colors cursor-pointer ${viewMode === 'grid' ? 'bg-primary/20 text-primary' : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-bright'}`} title="Grid view">
                           <span className="material-symbols-outlined text-[20px]">grid_view</span>
                         </button>
                         <button onClick={() => setViewMode('list')} className={`p-2 flex items-center justify-center border-l border-outline-variant/40 transition-colors cursor-pointer ${viewMode === 'list' ? 'bg-primary/20 text-primary' : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-bright'}`} title="List view">
                           <span className="material-symbols-outlined text-[20px]">view_list</span>
                         </button>
                      </div>
                    </div>
                 </div>

                 {/* Folders Section (Only in all-files) */}
                 {activeTab === 'all-files' && (
                   <div className="mb-space-xl">
                     <div className="flex items-center flex-wrap gap-2 text-primary text-sm font-semibold mb-3">
                       {breadcrumbs.map((crumb, idx) => (
                         <React.Fragment key={crumb.id || 'root'}>
                           {idx > 0 && <span className="text-outline-variant">/</span>}
                           <button 
                             onClick={() => navigateToFolder(crumb)}
                             className="hover:underline cursor-pointer"
                           >
                             {crumb.name}
                           </button>
                         </React.Fragment>
                       ))}
                     </div>
                     <div className="flex items-center justify-between mb-space-md">
                       <h3 className="font-headline-sm text-headline-sm font-semibold text-on-surface">Folders</h3>
                       <button 
                         onClick={() => setShowNewFolderModal(true)}
                         className="flex items-center gap-1 px-3 py-1.5 rounded-lg bg-surface-container-high hover:bg-surface-bright text-primary text-sm font-medium transition-colors cursor-pointer"
                       >
                         <span className="material-symbols-outlined text-[18px]">create_new_folder</span>
                         New Folder
                       </button>
                     </div>
                     <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-space-md">
                       {visibleFolders.map((folder) => (
                         <div 
                           key={folder.id} 
                           onClick={() => {
                             navigateToFolder(folder);
                             showToast(`Opened folder: ${folder.name}`, "info");
                           }}
                           className="flex items-center justify-between p-space-md rounded-2xl bg-surface-container-low/80 backdrop-blur border border-outline-variant/30 hover:bg-surface-container hover:shadow-md hover:border-outline-variant/50 transition-all cursor-pointer group"
                         >
                           <div className="flex items-center gap-space-md">
                             <div className="w-12 h-12 rounded-xl bg-primary-container/20 text-primary flex items-center justify-center group-hover:scale-105 transition-transform border border-primary/20">
                               <span className="material-symbols-outlined text-[24px]">folder</span>
                             </div>
                             <div className="flex flex-col">
                               <span className="font-body-lg text-on-surface font-medium group-hover:text-primary transition-colors">{folder.name}</span>
                               <span className="font-body-sm text-on-surface-variant">{files.filter(f => f.folder_id === folder.id).length} items</span>
                             </div>
                           </div>
                           <button 
                             onClick={(e) => handleOpenFolderMenu(e, folder)}
                             className="text-on-surface-variant hover:text-on-surface p-2 rounded-full hover:bg-surface-container-high transition-colors opacity-0 group-hover:opacity-100 cursor-pointer"
                             title="Folder Options"
                           >
                             <span className="material-symbols-outlined text-[20px]">more_vert</span>
                           </button>
                         </div>
                       ))}
                     </div>
                   </div>
                 )}

                 {/* Content Area */}
                 <div className="flex items-center justify-between mb-space-md">
                   <h3 className="font-headline-sm text-headline-sm font-semibold text-on-surface">
                     {activeTab === 'all-files' ? 'Files' : `${activeTab.replace('-', ' ')}`}
                     <span className="text-on-surface-variant text-sm font-normal ml-2">({filteredFiles.length})</span>
                   </h3>
                 </div>

                 {filteredFiles.length === 0 ? (
                   <div className="py-20 flex flex-col items-center justify-center text-center rounded-3xl bg-surface-container-low/40 border border-outline-variant/20 mb-8">
                     <div className="w-16 h-16 rounded-2xl bg-surface-container-high flex items-center justify-center text-outline mb-3">
                       <span className="material-symbols-outlined text-4xl">folder_open</span>
                     </div>
                     <h4 className="font-headline-sm text-on-surface font-semibold mb-1">No files found</h4>
                     <p className="text-body-sm text-on-surface-variant mb-4">No encrypted files match your current category or search query.</p>
                     <button 
                       onClick={() => setShowUploadModal(true)} 
                       className="flex items-center gap-2 bg-primary text-on-primary-fixed px-5 py-2.5 rounded-xl font-bold hover:bg-primary-fixed transition-colors shadow-md cursor-pointer"
                     >
                       <span className="material-symbols-outlined text-[18px]">add</span> Upload &amp; Encrypt File
                     </button>
                   </div>
                 ) : viewMode === 'grid' ? (
                   <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-space-md pb-space-xl">
                     {filteredFiles.map((file) => (
                       <div 
                         key={file.id} 
                         onClick={() => handlePreview(file)}
                         className="group cursor-pointer rounded-2xl border border-outline-variant/20 bg-surface-container-low/50 overflow-hidden hover:shadow-lg hover:border-outline-variant/50 transition-all"
                       >
                         <div className="aspect-square bg-surface-container-highest relative overflow-hidden flex items-center justify-center">
                           {file.metadata?.thumbnail_telegram_file_id ? (
                             <div className="w-full h-full relative group/v">
                               <img
                                 src={`/api/files/${file.id}/thumbnail?token=${getToken()}`}
                                 className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-500"
                                 alt={file.name}
                               />
                               {file.mime_type.startsWith('video/') && (
                                 <div className="absolute inset-0 bg-black/20 flex items-center justify-center">
                                   <span className="material-symbols-outlined text-4xl text-white drop-shadow-md">play_circle</span>
                                 </div>
                               )}
                             </div>
                           ) : file.mime_type.startsWith('image/') ? (
                             <img 
                               src={`/api/files/${file.id}/preview?token=${getToken()}`} 
                               onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = 'none'; }}
                               className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-500" 
                               alt={file.name} 
                             />
                           ) : file.mime_type.startsWith('video/') ? (
                              <div className="w-full h-full relative group/v bg-surface-container-low">
                                <VideoThumbnail file={file} className="w-full h-full" />
                               <div className="absolute inset-0 bg-black/40 flex items-center justify-center">
                                 <span className="material-symbols-outlined text-4xl text-primary">play_circle</span>
                               </div>
                             </div>
                           ) : (
                             <div className="w-full h-full flex items-center justify-center bg-surface-container/50">
                               <span className="material-symbols-outlined text-5xl text-outline-variant group-hover:text-primary transition-colors">{getFileIcon(file)}</span>
                             </div>
                           )}
                           <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity">
                             <button 
                               onClick={(e) => handleOpenMenu(e, file)}
                               className="w-8 h-8 rounded-full bg-surface-container-high/90 text-on-surface hover:bg-surface-bright flex items-center justify-center backdrop-blur shadow-sm cursor-pointer"
                               title="Options"
                             >
                               <span className="material-symbols-outlined text-[16px]">more_vert</span>
                             </button>
                           </div>
                         </div>
                         <div className="p-3">
                           <h4 className="font-bold text-on-surface text-sm truncate group-hover:text-primary transition-colors">{file.name}</h4>
                           <p className="text-on-surface-variant text-xs mt-1">
                             {formatBytes(file.size)} 
                             {file.metadata?.duration ? ` • ${Math.floor(file.metadata.duration / 60)}:${Math.floor(file.metadata.duration % 60).toString().padStart(2, '0')}` : ''}
                             {` • ${formatDate(file.created_at)}`}
                           </p>
                         </div>
                       </div>
                     ))}
                   </div>
                 ) : (
                   <div className="w-full bg-surface-container-low/30 backdrop-blur-sm rounded-3xl border border-outline-variant/20 overflow-hidden shadow-sm mb-8">
                     <table className="w-full text-left border-collapse">
                       <thead>
                         <tr className="border-b border-outline-variant/20 text-on-surface-variant font-label-mono-sm uppercase text-xs tracking-wider">
                           <th className="py-4 pl-6 w-12 font-medium"></th>
                           <th className="py-4 font-medium">Name</th>
                           <th className="py-4 font-medium">Size</th>
                           <th className="py-4 font-medium">Modified</th>
                           <th className="py-4 pr-6 text-right font-medium">Actions</th>
                         </tr>
                       </thead>
                       <tbody>
                         {filteredFiles.map((file) => (
                           <tr 
                             key={file.id} 
                             onClick={() => handlePreview(file)}
                             className="group hover:bg-surface-container-high transition-colors cursor-pointer border-b border-outline-variant/10 last:border-0"
                           >
                             <td className="py-4 pl-6 text-on-surface-variant w-12">
                               {file.metadata?.thumbnail_telegram_file_id ? (
                                 <img 
                                   src={`/api/files/${file.id}/thumbnail?token=${getToken()}`} 
                                   className="w-8 h-8 object-cover rounded shadow-sm"
                                   alt={file.name} 
                                 />
                               ) : (
                                 <span className="material-symbols-outlined text-[20px] text-primary">{getFileIcon(file)}</span>
                               )}
                             </td>
                             <td className="py-4 font-medium text-on-surface text-sm group-hover:text-primary transition-colors">
                               {file.name}
                             </td>
                             <td className="py-4 text-on-surface-variant text-sm font-label-mono-sm">{formatBytes(file.size)}</td>
                             <td className="py-4 text-on-surface-variant text-sm">{formatDate(file.created_at)}</td>
                             <td className="py-4 pr-6 text-right">
                               <button 
                                 onClick={(e) => handleOpenMenu(e, file)}
                                 className="text-on-surface-variant hover:text-primary transition-colors p-1.5 rounded-lg hover:bg-surface-container cursor-pointer"
                                 title="File Options"
                               >
                                 <span className="material-symbols-outlined text-[18px]">more_vert</span>
                               </button>
                             </td>
                           </tr>
                         ))}
                       </tbody>
                     </table>
                   </div>
                 )}
              </div>
            ) : (
              <div className="flex-1 flex flex-col items-center justify-center py-40">
                <span className="material-symbols-outlined text-6xl text-outline mb-4">construction</span>
                <h2 className="font-headline-md text-headline-md text-on-surface mb-2">Work in Progress</h2>
                <p className="text-on-surface-variant">The {activeTab} view is not yet implemented.</p>
              </div>
            )}
            {/*  Section 4: Dynamic Bottom Floating Audio Bar  */}
            {hasActiveAudioSession && (
              <>
                {/* Embedded HTML5 Audio Stream - Kept mounted continuously during playback session */}
                <audio 
                  ref={audioRef}
                  src={currentTrack.fileId ? `/api/files/${currentTrack.fileId}/stream?token=${getToken()}` : ''}
                  preload="auto"
                  playsInline
                  onTimeUpdate={(e) => setAudioProgressSec(Math.floor(e.currentTarget.currentTime))}
                  onLoadedMetadata={(e) => {
                    const dur = Math.floor(e.currentTarget.duration);
                    if (!isNaN(dur) && dur > 0) {
                      setTrackDurationSec(dur);
                    }
                    if (isPlayingAudio) {
                      e.currentTarget.play().catch(err => {
                        if (err.name !== 'AbortError') console.warn(err);
                      });
                    }
                  }}
                  onDurationChange={(e) => {
                    const dur = Math.floor(e.currentTarget.duration);
                    if (!isNaN(dur) && dur > 0) {
                      setTrackDurationSec(dur);
                    }
                  }}
                  onEnded={() => {
                    if (isRepeat) {
                      if (audioRef.current) {
                        audioRef.current.currentTime = 0;
                        audioRef.current.play().catch(console.error);
                      }
                    } else if (playlist.length > 1) {
                      handleNextTrack();
                    } else {
                      setIsPlayingAudio(false);
                      setAudioProgressSec(0);
                    }
                  }}
                />

                {!isDockMinimized ? (
                  <aside
                    className={`fixed bottom-4 right-8 z-40 rounded-2xl bg-surface-container-high/95 backdrop-blur-2xl p-space-sm px-space-md shadow-2xl transition-all duration-300 border border-outline-variant/50 ${isSidebarOpen ? 'lg:left-[340px] left-8' : 'left-8'}`}
                    id="audio-player-dock"
                  >

                {/* Queue Popover Drawer */}
                {showQueueDrawer && (
                  <div 
                    className="absolute bottom-full right-0 mb-3 w-80 sm:w-96 max-h-[420px] rounded-2xl bg-surface-container-high/98 backdrop-blur-2xl border border-outline-variant/60 shadow-2xl p-4 flex flex-col z-50 animate-in fade-in slide-in-from-bottom-2 duration-200"
                    onClick={(e) => e.stopPropagation()}
                  >
                    <div className="flex items-center justify-between pb-3 border-b border-outline-variant/30 mb-2">
                      <div className="flex items-center gap-2">
                        <span className="material-symbols-outlined text-primary text-[20px]">queue_music</span>
                        <h3 className="font-bold text-sm text-on-surface">Playback Queue</h3>
                        <span className="text-xs font-mono text-on-surface-variant">({playlist.length})</span>
                      </div>
                      <div className="flex items-center gap-1">
                        {playlist.length > 0 && (
                          <button 
                            onClick={handleClearQueue}
                            className="text-xs text-on-surface-variant hover:text-rose-400 px-2 py-1 rounded transition-colors cursor-pointer"
                          >
                            Clear
                          </button>
                        )}
                        <button 
                          onClick={() => setShowQueueDrawer(false)}
                          className="text-on-surface-variant hover:text-on-surface p-1 rounded-lg cursor-pointer"
                        >
                          <span className="material-symbols-outlined text-[18px]">close</span>
                        </button>
                      </div>
                    </div>

                    {playlist.length === 0 ? (
                      <div className="py-8 text-center text-on-surface-variant text-xs">
                        <span className="material-symbols-outlined text-2xl mb-1 text-outline">queue</span>
                        <p>Your queue is empty.</p>
                        <p className="text-[10px] text-outline mt-0.5">Click any song or "Play All" to add tracks.</p>
                      </div>
                    ) : (
                      <div className="overflow-y-auto max-h-[320px] space-y-1 pr-1 custom-scrollbar">
                        {playlist.map((trackItem, idx) => {
                          const isThisTrack = idx === currentTrackIndex;
                          const trackFileId = trackItem.id || trackItem.fileId || '';
                          const rawName = trackItem.name || trackItem.title || 'Track';
                          const info = parseAudioInfo(rawName, trackItem.metadata?.artist, trackItem.metadata?.title);

                          return (
                            <div
                              key={`${trackFileId}-${idx}`}
                              onClick={() => {
                                setCurrentTrackIndex(idx);
                                setAudioProgressSec(0);
                                setIsPlayingAudio(true);
                              }}
                              className={`flex items-center justify-between p-2 rounded-xl transition-all cursor-pointer group text-xs ${
                                isThisTrack 
                                  ? 'bg-primary/15 text-primary border border-primary/30 font-semibold' 
                                  : 'hover:bg-surface-container text-on-surface border border-transparent'
                              }`}
                            >
                              <div className="flex items-center gap-2.5 min-w-0 flex-1">
                                <div className="w-6 text-center text-on-surface-variant font-mono text-[10px] flex-shrink-0">
                                  {isThisTrack && isPlayingAudio ? (
                                    <span className="material-symbols-outlined text-primary text-[16px] animate-pulse">volume_up</span>
                                  ) : (
                                    idx + 1
                                  )}
                                </div>
                                <div className="min-w-0 flex-1">
                                  <p className={`truncate ${isThisTrack ? 'text-primary font-bold' : 'text-on-surface'}`}>{info.title}</p>
                                  <p className="text-[10px] text-on-surface-variant truncate">{info.artist}</p>
                                </div>
                              </div>
                              <div className="flex items-center gap-2 flex-shrink-0 ml-2">
                                <button
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    handleRemoveFromQueue(idx);
                                  }}
                                  className="text-on-surface-variant hover:text-rose-400 hover:bg-rose-500/10 p-1 rounded-lg transition-colors cursor-pointer"
                                  title="Remove from queue"
                                  aria-label="Remove from queue"
                                >
                                  <span className="material-symbols-outlined text-[16px]">close</span>
                                </button>
                              </div>
                            </div>
                          );
                        })}
                      </div>
                    )}
                  </div>
                )}

                <div className="flex items-center justify-between gap-space-md">
                  {/* Left: Track Artwork & Info */}
                  <div className="flex items-center gap-space-sm min-w-0 w-64">
                    <div className="relative w-12 h-12 rounded-xl overflow-hidden flex-shrink-0 shadow-md bg-surface-container-high border border-outline-variant/40">
                      {currentTrack.cover ? (
                        <img
                          src={currentTrack.cover}
                          className="absolute inset-0 w-full h-full object-cover z-10"
                          alt={currentTrack.title}
                        />
                      ) : (
                        <div className="absolute inset-0 flex items-center justify-center z-0">
                          <span className={`material-symbols-outlined text-primary text-lg ${isPlayingAudio ? 'animate-pulse' : ''}`}>
                            {isPlayingAudio ? 'volume_up' : 'music_note'}
                          </span>
                        </div>
                      )}
                    </div>
                    <div className="flex flex-col min-w-0">
                      <div className="flex items-center gap-1.5">
                        <span className="font-headline-sm text-headline-sm text-on-surface truncate font-semibold">
                          {currentTrack.title}
                        </span>
                        <span className="font-label-mono-sm text-label-mono-sm px-1.5 py-0.2 rounded bg-surface-container text-primary text-[10px] border border-outline-variant/40 uppercase flex-shrink-0">
                          {currentTrack.format}
                        </span>
                      </div>
                      <span className="font-body-sm text-body-sm text-on-surface-variant truncate">
                        {currentTrack.artist}
                      </span>
                    </div>
                  </div>

                  {/* Center: Controls & Dynamic Progress */}
                  <div className="flex-1 max-w-2xl flex flex-col items-center gap-1">
                    <div className="flex items-center gap-space-md">
                      <button
                        aria-label="Shuffle"
                        className={`p-1.5 rounded-lg transition-colors cursor-pointer ${isShuffle ? 'text-primary bg-primary/10' : 'text-on-surface-variant hover:text-on-surface'}`}
                        type="button"
                        onClick={() => {
                          setIsShuffle(!isShuffle);
                          showToast(isShuffle ? "Shuffle disabled" : "Shuffle enabled", "info");
                        }}
                        title={isShuffle ? "Shuffle On" : "Shuffle Off"}
                      >
                        <span className="material-symbols-outlined text-[20px]">shuffle</span>
                      </button>
                      <button
                        aria-label="Previous track"
                        className="text-on-surface-variant hover:text-on-surface transition-colors cursor-pointer p-1 rounded-lg"
                        type="button"
                        onClick={handlePrevTrack}
                        title="Previous Track"
                      >
                        <span className="material-symbols-outlined text-[22px]">skip_previous</span>
                      </button>
                      <button
                        aria-label="Pause or play"
                        className="w-10 h-10 rounded-full bg-primary text-on-primary flex items-center justify-center hover:scale-105 active:scale-95 transition-all shadow-[0_0_12px_rgba(252,186,105,0.35)] cursor-pointer"
                        id="toggle-play-btn"
                        type="button"
                        onClick={() => setIsPlayingAudio(!isPlayingAudio)}
                        title={isPlayingAudio ? "Pause" : "Play"}
                      >
                        <span
                          className="material-symbols-outlined text-2xl"
                          id="toggle-play-icon"
                          style={{ fontVariationSettings: "'FILL' 1" }}
                        >{isPlayingAudio ? 'pause' : 'play_arrow'}</span>
                      </button>
                      <button
                        aria-label="Next track"
                        className="text-on-surface-variant hover:text-on-surface transition-colors cursor-pointer p-1 rounded-lg"
                        type="button"
                        onClick={handleNextTrack}
                        title="Next Track"
                      >
                        <span className="material-symbols-outlined text-[22px]">skip_next</span>
                      </button>
                      <button
                        aria-label="Repeat"
                        className={`p-1.5 rounded-lg transition-colors cursor-pointer ${isRepeat ? 'text-primary bg-primary/10' : 'text-on-surface-variant hover:text-on-surface'}`}
                        type="button"
                        onClick={() => {
                          setIsRepeat(!isRepeat);
                          showToast(isRepeat ? "Repeat disabled" : "Repeat enabled", "info");
                        }}
                        title={isRepeat ? "Repeat On" : "Repeat Off"}
                      >
                        <span className="material-symbols-outlined text-[20px]">repeat</span>
                      </button>
                    </div>

                    {/* Progress Slider */}
                    <div className="w-full flex items-center gap-space-sm">
                      <span className="font-label-mono-sm text-label-mono-sm text-on-surface-variant w-11 text-right font-mono text-xs">
                        {formatSec(audioProgressSec)}
                      </span>
                      <div 
                        className="flex-1 bg-surface-container-highest rounded-full h-1.5 cursor-pointer relative group"
                        onClick={(e) => {
                          if (!currentTrack.durationSec || currentTrack.durationSec <= 0) return;
                          const rect = e.currentTarget.getBoundingClientRect();
                          const pct = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
                          const newTime = Math.round(pct * currentTrack.durationSec);
                          setAudioProgressSec(newTime);
                          if (audioRef.current) {
                            audioRef.current.currentTime = newTime;
                          }
                        }}
                      >
                        <div
                          className="bg-primary h-1.5 rounded-full relative transition-[width] duration-75"
                          style={{ width: `${currentTrack.durationSec > 0 ? Math.min(100, (audioProgressSec / currentTrack.durationSec) * 100) : 0}%` }}
                        >
                          <span className="absolute right-0 top-1/2 -translate-y-1/2 w-3.5 h-3.5 bg-primary-fixed rounded-full shadow-md opacity-0 group-hover:opacity-100 transition-opacity"></span>
                        </div>
                      </div>
                      <span className="font-label-mono-sm text-label-mono-sm text-on-surface-variant w-11 font-mono text-xs">
                        {currentTrack.durationSec > 0 ? formatSec(currentTrack.durationSec) : '--:--'}
                      </span>
                    </div>
                  </div>

                  {/* Right: Volume, Queue, Minimize, Close */}
                  <div className="flex items-center gap-space-sm w-64 justify-end">
                    {/* Volume Slider */}
                    <div className="flex items-center gap-1.5 w-24">
                      <button 
                        onClick={() => setIsMuted(!isMuted)} 
                        className="text-on-surface-variant hover:text-on-surface p-0.5 cursor-pointer"
                        title={isMuted ? "Unmute" : "Mute"}
                      >
                        <span className="material-symbols-outlined text-[18px]">{isMuted ? 'volume_off' : volume === 0 ? 'volume_mute' : volume < 0.5 ? 'volume_down' : 'volume_up'}</span>
                      </button>
                      <div 
                        className="flex-1 bg-surface-container-highest rounded-full h-1 cursor-pointer"
                        onClick={(e) => {
                          const rect = e.currentTarget.getBoundingClientRect();
                          const val = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
                          setVolume(val);
                          setIsMuted(false);
                        }}
                      >
                        <div
                          className="bg-primary h-1 rounded-full transition-all"
                          style={{ width: `${isMuted ? 0 : volume * 100}%` }}
                        ></div>
                      </div>
                    </div>

                    {/* Queue Drawer Button */}
                    <button
                      aria-label="Queue"
                      className={`p-1.5 rounded-lg transition-colors cursor-pointer relative ${showQueueDrawer ? 'text-primary bg-primary/10' : 'text-on-surface-variant hover:text-on-surface'}`}
                      type="button"
                      onClick={() => setShowQueueDrawer(!showQueueDrawer)}
                      title={`Queue (${playlist.length})`}
                    >
                      <span className="material-symbols-outlined text-[20px]">queue_music</span>
                      {playlist.length > 0 && (
                        <span className="absolute -top-1 -right-1 w-4 h-4 rounded-full bg-primary text-on-primary text-[9px] font-bold flex items-center justify-center">
                          {playlist.length > 99 ? '99+' : playlist.length}
                        </span>
                      )}
                    </button>

                    {/* Minimize Player Button */}
                    <button
                      aria-label="Minimize player"
                      className="p-1.5 text-on-surface-variant hover:text-on-surface transition-colors rounded-lg hover:bg-surface-container-highest cursor-pointer ml-1"
                      id="minimize-player-btn"
                      type="button"
                      onClick={() => setIsDockMinimized(true)}
                      title="Minimize Audio Dock"
                    >
                      <span className="material-symbols-outlined text-[20px]">keyboard_arrow_down</span>
                    </button>

                    {/* Close Player Button */}
                    <button
                      aria-label="Close player"
                      className="p-1.5 text-on-surface-variant hover:text-rose-400 transition-colors rounded-lg hover:bg-rose-500/10 cursor-pointer"
                      id="dismiss-player-btn"
                      type="button"
                      onClick={() => { setHasActiveAudioSession(false); setIsPlayingAudio(false); setShowQueueDrawer(false); }}
                      title="Close Audio Dock"
                    >
                      <span className="material-symbols-outlined text-[20px]">close</span>
                    </button>
                  </div>
                </div>
              </aside>
            ) : (
              <div 
                onClick={() => setIsDockMinimized(false)}
                className="fixed bottom-6 right-8 z-40 rounded-full bg-surface-container-high/95 backdrop-blur-xl p-2 pr-4 shadow-2xl border border-primary/30 flex items-center gap-3 cursor-pointer hover:border-primary transition-all group"
                title="Restore Audio Dock"
              >
                {currentTrack.cover ? (
                  <img 
                    src={currentTrack.cover} 
                    className={`w-9 h-9 rounded-full object-cover ${isPlayingAudio ? 'animate-spin' : ''}`} 
                    alt="" 
                    style={{ animationDuration: '6s' }} 
                  />
                ) : (
                  <div className={`w-9 h-9 rounded-full bg-surface-container-high flex items-center justify-center ${isPlayingAudio ? 'animate-spin' : ''}`} style={{ animationDuration: '6s' }}>
                    <span className="material-symbols-outlined text-[16px] text-primary">music_note</span>
                  </div>
                )}
                <div className="flex flex-col">
                  <span className="text-xs font-bold text-on-surface line-clamp-1">{currentTrack.title}</span>
                  <span className="text-[10px] text-on-surface-variant line-clamp-1">{currentTrack.artist}</span>
                </div>
                <button 
                  onClick={(e) => { e.stopPropagation(); setIsPlayingAudio(!isPlayingAudio); }}
                  className="w-7 h-7 rounded-full bg-primary text-on-primary flex items-center justify-center cursor-pointer shadow-sm hover:scale-105"
                >
                  <span className="material-symbols-outlined text-sm">{isPlayingAudio ? 'pause' : 'play_arrow'}</span>
                </button>
                <span className="material-symbols-outlined text-outline group-hover:text-primary text-sm">open_in_full</span>
                <div className="w-px h-4 bg-outline-variant/40 mx-1"></div>
                <button 
                  onClick={(e) => { e.stopPropagation(); setHasActiveAudioSession(false); setIsPlayingAudio(false); }}
                  className="text-on-surface-variant hover:text-on-surface cursor-pointer flex items-center justify-center"
                  title="Close Player"
                >
                  <span className="material-symbols-outlined text-[16px]">close</span>
                </button>
              </div>
            )}
          </>
        )}
          </div>
        </main>
      </div>

      {/* ========================================================================= */}
      {/* ALL POPUP WINDOWS & MODALS */}
      {/* ========================================================================= */}

      {/* 1. Full Interactive File Preview Modal */}
      {previewFile && (
        <FilePreview 
          file={previewFile} 
          onClose={() => setPreviewFile(null)} 
          onDockToBottomPlayer={handleDockToBottomPlayer}
        />
      )}

      {/* 2. Upload Modal */}
      {showUploadModal && (
        <UploadModal 
          folderId={currentFolderId || undefined}
          onClose={() => setShowUploadModal(false)}
          onStartUpload={(filesToUpload) => {
            startUploads(filesToUpload, currentFolderId || undefined);
            showToast(`Queued ${filesToUpload.length} ${filesToUpload.length === 1 ? 'file' : 'files'} for upload`, 'info');
          }}
        />
      )}

      {/* 3. Floating 3-Dots Context Menu */}
      {activeMenu && (
        <div 
          className="fixed inset-0 z-50 bg-black/10"
          onClick={() => setActiveMenu(null)}
          onContextMenu={(e) => { e.preventDefault(); setActiveMenu(null); }}
        >
          <div 
            ref={menuCallbackRef}
            className="fixed z-50 min-w-[210px] max-w-[280px] max-h-[calc(100vh-24px)] overflow-y-auto custom-scrollbar rounded-2xl bg-surface-container-high/95 backdrop-blur-2xl p-1.5 shadow-2xl border border-outline-variant/40 animate-in fade-in zoom-in-95 duration-100"
            style={{ 
              left: `${Math.max(12, Math.min(window.innerWidth - 230, activeMenu.x))}px`, 
              top: `${Math.max(12, Math.min(window.innerHeight - 460, activeMenu.y))}px` 
            }}
            onClick={(e) => e.stopPropagation()}
          >
            {!activeMenu.isFolder && activeMenu.file ? (
              <>
                <div className="px-3 py-1.5 border-b border-outline-variant/30 mb-1">
                  <p className="text-xs font-bold text-on-surface truncate">{activeMenu.file.name}</p>
                  <p className="text-[10px] text-on-surface-variant">{formatBytes(activeMenu.file.size)}</p>
                </div>
                <button 
                  onClick={() => { const f = activeMenu.file!; setActiveMenu(null); handlePreview(f); }}
                  className="w-full flex items-center gap-2.5 px-3 py-2 text-sm font-medium text-on-surface hover:bg-surface-container-highest hover:text-primary rounded-xl transition-colors text-left cursor-pointer"
                >
                  <span className="material-symbols-outlined text-[18px]">visibility</span>
                  Preview / Open
                </button>
                {(activeMenu.file.mime_type.startsWith('video/') || /\.(mp4|mkv|webm|mov|avi)$/i.test(activeMenu.file.name)) && (
                  <button 
                    onClick={() => { 
                      const f = activeMenu.file!; 
                      setActiveMenu(null); 
                      handleToggleCinema(f);
                    }}
                    className="w-full flex items-center gap-2.5 px-3 py-2 text-sm font-medium text-primary hover:bg-surface-container-highest rounded-xl transition-colors text-left cursor-pointer"
                  >
                    <span className="material-symbols-outlined text-[18px]">
                      {activeMenu.file.is_cinema ? 'movie_edit' : 'theaters'}
                    </span>
                    {activeMenu.file.is_cinema ? 'Remove from Cinema' : 'Add to Cinema'}
                  </button>
                )}
                {(activeMenu.file.mime_type.startsWith('audio/') || /\.(mp3|wav|flac|aac|ogg|m4a|wma)$/i.test(activeMenu.file.name)) && (
                  <>
                    <button 
                      onClick={() => { 
                        const f = activeMenu.file!; 
                        setActiveMenu(null); 
                        handleDockToBottomPlayer(f);
                      }}
                      className="w-full flex items-center gap-2.5 px-3 py-2 text-sm font-medium text-primary hover:bg-surface-container-highest rounded-xl transition-colors text-left cursor-pointer"
                    >
                      <span className="material-symbols-outlined text-[18px]">play_circle</span>
                      Play Now
                    </button>
                    <button 
                      onClick={() => { 
                        const f = activeMenu.file!; 
                        setActiveMenu(null); 
                        handlePlayNext(f);
                      }}
                      className="w-full flex items-center gap-2.5 px-3 py-2 text-sm font-medium text-on-surface hover:bg-surface-container-highest rounded-xl transition-colors text-left cursor-pointer"
                    >
                      <span className="material-symbols-outlined text-[18px]">playlist_play</span>
                      Play Next
                    </button>
                    {playlist.some(p => (p.id || p.fileId) === activeMenu.file!.id) ? (
                      <button 
                        onClick={() => { 
                          const f = activeMenu.file!; 
                          setActiveMenu(null); 
                          handleRemoveFileFromQueue(f.id);
                        }}
                        className="w-full flex items-center gap-2.5 px-3 py-2 text-sm font-medium text-rose-400 hover:bg-rose-500/10 rounded-xl transition-colors text-left cursor-pointer"
                      >
                        <span className="material-symbols-outlined text-[18px]">playlist_remove</span>
                        Remove from Queue
                      </button>
                    ) : (
                      <button 
                        onClick={() => { 
                          const f = activeMenu.file!; 
                          setActiveMenu(null); 
                          handleAddToQueue(f);
                        }}
                        className="w-full flex items-center gap-2.5 px-3 py-2 text-sm font-medium text-on-surface hover:bg-surface-container-highest rounded-xl transition-colors text-left cursor-pointer"
                      >
                        <span className="material-symbols-outlined text-[18px]">queue_music</span>
                        Add to Queue
                      </button>
                    )}
                  </>
                )}
                <button 
                  onClick={() => { const f = activeMenu.file!; setActiveMenu(null); handleDownload(f); }}
                  className="w-full flex items-center gap-2.5 px-3 py-2 text-sm font-medium text-on-surface hover:bg-surface-container-highest rounded-xl transition-colors text-left cursor-pointer"
                >
                  <span className="material-symbols-outlined text-[18px]">download</span>
                  Download File
                </button>
                <button 
                  onClick={() => { const f = activeMenu.file!; setActiveMenu(null); handleShare(f); }}
                  className="w-full flex items-center gap-2.5 px-3 py-2 text-sm font-medium text-on-surface hover:bg-surface-container-highest rounded-xl transition-colors text-left cursor-pointer"
                >
                  <span className="material-symbols-outlined text-[18px]">share</span>
                  Share Stream Link
                </button>
                <button 
                  onClick={() => { const f = activeMenu.file!; setActiveMenu(null); handleToggleFavorite(f.id); }}
                  className="w-full flex items-center gap-2.5 px-3 py-2 text-sm font-medium text-on-surface hover:bg-surface-container-highest rounded-xl transition-colors text-left cursor-pointer"
                >
                  <span className="material-symbols-outlined text-[18px]">{favorites.has(activeMenu.file.id) ? 'heart_minus' : 'favorite'}</span>
                  {favorites.has(activeMenu.file.id) ? 'Remove Favorite' : 'Add to Favorites'}
                </button>
                <button 
                  onClick={() => { const f = activeMenu.file!; setActiveMenu(null); setDetailsTarget(f); }}
                  className="w-full flex items-center gap-2.5 px-3 py-2 text-sm font-medium text-on-surface hover:bg-surface-container-highest rounded-xl transition-colors text-left cursor-pointer"
                >
                  <span className="material-symbols-outlined text-[18px]">info</span>
                  File Details &amp; Cipher
                </button>
                <button 
                  onClick={() => { const f = activeMenu.file!; setActiveMenu(null); setRenameTarget({ item: f, isFolder: false, name: f.name }); }}
                  className="w-full flex items-center gap-2.5 px-3 py-2 text-sm font-medium text-on-surface hover:bg-surface-container-highest rounded-xl transition-colors text-left cursor-pointer"
                >
                  <span className="material-symbols-outlined text-[18px]">edit</span>
                  Rename
                </button>
                <div className="h-px bg-outline-variant/30 my-1"></div>
                <button 
                  onClick={() => { const f = activeMenu.file!; setActiveMenu(null); setDeleteTarget({ item: f, isFolder: false }); }}
                  className="w-full flex items-center gap-2.5 px-3 py-2 text-sm font-medium text-error hover:bg-error-container/20 rounded-xl transition-colors text-left cursor-pointer"
                >
                  <span className="material-symbols-outlined text-[18px]">delete</span>
                  Delete File
                </button>
              </>
            ) : activeMenu.folder ? (
              <>
                <div className="px-3 py-1.5 border-b border-outline-variant/30 mb-1">
                  <p className="text-xs font-bold text-on-surface truncate">{activeMenu.folder.name}</p>
                  <p className="text-[10px] text-on-surface-variant">Folder</p>
                </div>
                <button 
                  onClick={() => { const fol = activeMenu.folder!; setActiveMenu(null); navigateToFolder(fol); showToast(`Opened ${fol.name}`, "info"); }}
                  className="w-full flex items-center gap-2.5 px-3 py-2 text-sm font-medium text-on-surface hover:bg-surface-container-highest hover:text-primary rounded-xl transition-colors text-left cursor-pointer"
                >
                  <span className="material-symbols-outlined text-[18px]">folder_open</span>
                  Open Folder
                </button>
                <button 
                  onClick={() => { const fol = activeMenu.folder!; setActiveMenu(null); setRenameTarget({ item: fol, isFolder: true, name: fol.name }); }}
                  className="w-full flex items-center gap-2.5 px-3 py-2 text-sm font-medium text-on-surface hover:bg-surface-container-highest rounded-xl transition-colors text-left cursor-pointer"
                >
                  <span className="material-symbols-outlined text-[18px]">edit</span>
                  Rename Folder
                </button>
                <div className="h-px bg-outline-variant/30 my-1"></div>
                <button 
                  onClick={() => { const fol = activeMenu.folder!; setActiveMenu(null); setDeleteTarget({ item: fol, isFolder: true }); }}
                  className="w-full flex items-center gap-2.5 px-3 py-2 text-sm font-medium text-error hover:bg-error-container/20 rounded-xl transition-colors text-left cursor-pointer"
                >
                  <span className="material-symbols-outlined text-[18px]">delete</span>
                  Delete Folder
                </button>
              </>
            ) : null}
          </div>
        </div>
      )}

      {/* 4. Rename Modal */}
      {renameTarget && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="w-full max-w-md rounded-3xl bg-surface-container-high border border-outline-variant/40 p-space-lg shadow-2xl animate-in zoom-in-95">
            <h3 className="font-headline-sm text-headline-sm font-bold text-on-surface mb-2">
              Rename {renameTarget.isFolder ? 'Folder' : 'File'}
            </h3>
            <p className="text-body-sm text-on-surface-variant mb-4">
              Enter a new name for "{renameTarget.item.name}".
            </p>
            <input 
              type="text"
              autoFocus
              value={renameTarget.name}
              onChange={(e) => setRenameTarget({ ...renameTarget, name: e.target.value })}
              onKeyDown={(e) => { if (e.key === 'Enter') handleRenameSubmit(); if (e.key === 'Escape') setRenameTarget(null); }}
              className="w-full bg-surface-container-low border border-outline-variant/40 rounded-xl px-4 py-2.5 text-on-surface text-sm outline-none focus:border-primary focus:ring-1 focus:ring-primary mb-6"
            />
            <div className="flex items-center justify-end gap-3">
              <button 
                onClick={() => setRenameTarget(null)}
                className="px-4 py-2 rounded-xl text-sm font-semibold text-on-surface-variant hover:bg-surface-container transition-colors cursor-pointer"
              >
                Cancel
              </button>
              <button 
                onClick={handleRenameSubmit}
                className="px-5 py-2 rounded-xl text-sm font-semibold bg-primary text-on-primary-fixed hover:bg-primary-fixed transition-colors shadow-md cursor-pointer"
              >
                Save
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 5. File Details / Inspector Modal */}
      {detailsTarget && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="w-full max-w-lg rounded-3xl bg-surface-container-high border border-outline-variant/40 p-space-lg shadow-2xl animate-in zoom-in-95">
            <div className="flex items-center justify-between mb-4">
              <div className="flex items-center gap-2">
                <span className="material-symbols-outlined text-primary text-2xl">shield_lock</span>
                <h3 className="font-headline-sm text-headline-sm font-bold text-on-surface">File Inspector &amp; Cipher</h3>
              </div>
              <button 
                onClick={() => setDetailsTarget(null)}
                className="w-8 h-8 rounded-full hover:bg-surface-container text-on-surface-variant flex items-center justify-center cursor-pointer"
              >
                <span className="material-symbols-outlined text-[18px]">close</span>
              </button>
            </div>
            
            <div className="space-y-3 mb-6 bg-surface-container-low p-4 rounded-2xl border border-outline-variant/30 text-sm">
              <div className="flex justify-between py-1 border-b border-outline-variant/20">
                <span className="text-on-surface-variant font-medium">File Name:</span>
                <span className="text-on-surface font-semibold truncate max-w-[260px]">{detailsTarget.name}</span>
              </div>
              <div className="flex justify-between py-1 border-b border-outline-variant/20">
                <span className="text-on-surface-variant font-medium">Exact Size:</span>
                <span className="text-on-surface font-mono">{formatBytes(detailsTarget.size)} ({detailsTarget.size.toLocaleString()} bytes)</span>
              </div>
              <div className="flex justify-between py-1 border-b border-outline-variant/20">
                <span className="text-on-surface-variant font-medium">MIME Type:</span>
                <span className="text-on-surface font-mono text-xs">{detailsTarget.mime_type}</span>
              </div>
              <div className="flex justify-between py-1 border-b border-outline-variant/20">
                <span className="text-on-surface-variant font-medium">Telegram Shards:</span>
                <span className="text-primary font-semibold">{detailsTarget.chunk_count} chunks (20 MB / shard)</span>
              </div>
              <div className="flex justify-between py-1 border-b border-outline-variant/20">
                <span className="text-on-surface-variant font-medium">Cipher Algorithm:</span>
                <span className="text-on-surface font-mono text-xs text-primary">AES-256-GCM + Poly1305</span>
              </div>
              <div className="flex justify-between py-1 border-b border-outline-variant/20">
                <span className="text-on-surface-variant font-medium">Salt Nonce:</span>
                <span className="text-on-surface font-mono text-xs text-outline">{detailsTarget.encryption_salt}</span>
              </div>
              <div className="flex justify-between py-1 border-b border-outline-variant/20">
                <span className="text-on-surface-variant font-medium">Storage Location:</span>
                <span className="text-on-surface">Telegram MTProto Cluster (DC 4)</span>
              </div>
              <div className="flex justify-between py-1">
                <span className="text-on-surface-variant font-medium">Uploaded At:</span>
                <span className="text-on-surface">{new Date(detailsTarget.created_at).toLocaleString()}</span>
              </div>
            </div>

            <div className="flex items-center justify-between">
              <button 
                onClick={() => {
                  navigator.clipboard.writeText(JSON.stringify(detailsTarget, null, 2));
                  showToast("File telemetry copied to clipboard", "success");
                }}
                className="flex items-center gap-1.5 text-xs text-primary font-semibold hover:underline cursor-pointer"
              >
                <span className="material-symbols-outlined text-[16px]">content_copy</span>
                Copy JSON Metadata
              </button>
              <div className="flex gap-2">
                <button 
                  onClick={() => { const f = detailsTarget; setDetailsTarget(null); handlePreview(f); }}
                  className="px-4 py-2 rounded-xl text-sm font-semibold bg-surface-container hover:bg-surface-bright text-on-surface transition-colors cursor-pointer"
                >
                  Open Preview
                </button>
                <button 
                  onClick={() => setDetailsTarget(null)}
                  className="px-5 py-2 rounded-xl text-sm font-semibold bg-primary text-on-primary-fixed hover:bg-primary-fixed transition-colors cursor-pointer"
                >
                  Done
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* 6. Delete Confirmation Modal */}
      {deleteTarget && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="w-full max-w-md rounded-3xl bg-surface-container-high border border-outline-variant/40 p-space-lg shadow-2xl animate-in zoom-in-95">
            <div className="w-12 h-12 rounded-2xl bg-error/15 text-error flex items-center justify-center mb-4">
              <span className="material-symbols-outlined text-2xl">delete_forever</span>
            </div>
            <h3 className="font-headline-sm text-headline-sm font-bold text-on-surface mb-2">
              Delete {deleteTarget.isFolder ? 'Folder' : 'File'}?
            </h3>
            <p className="text-body-sm text-on-surface-variant mb-6">
              Are you sure you want to permanently delete <strong className="text-on-surface">"{deleteTarget.item.name}"</strong>? All encrypted chunks will be immediately purged from the Telegram cloud storage cluster.
            </p>
            <div className="flex items-center justify-end gap-3">
              <button 
                onClick={() => setDeleteTarget(null)}
                className="px-4 py-2 rounded-xl text-sm font-semibold text-on-surface-variant hover:bg-surface-container transition-colors cursor-pointer"
              >
                Cancel
              </button>
              <button 
                onClick={handleDeleteConfirm}
                className="px-5 py-2 rounded-xl text-sm font-semibold bg-error text-on-error hover:bg-error/90 transition-colors shadow-md cursor-pointer"
              >
                Delete Permanently
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 7. Share Link Modal */}
      {shareTarget && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="w-full max-w-md rounded-3xl bg-surface-container-high border border-outline-variant/40 p-space-lg shadow-2xl animate-in zoom-in-95">
            <div className="flex items-center justify-between mb-4">
              <div className="flex items-center gap-2">
                <span className="material-symbols-outlined text-primary text-2xl">share</span>
                <h3 className="font-headline-sm text-headline-sm font-bold text-on-surface">Share File Stream</h3>
              </div>
              <button onClick={() => setShareTarget(null)} className="w-8 h-8 rounded-full hover:bg-surface-container text-on-surface-variant flex items-center justify-center cursor-pointer">
                <span className="material-symbols-outlined text-[18px]">close</span>
              </button>
            </div>
            <p className="text-body-sm text-on-surface-variant mb-3">
              Anyone with this encrypted streaming link can stream or download <strong>"{shareTarget.name}"</strong>.
            </p>
            <div className="flex items-center gap-2 bg-surface-container-low p-2 rounded-xl border border-outline-variant/40 mb-6">
              <input 
                type="text" 
                readOnly 
                value={`${window.location.origin}/api/files/${shareTarget.id}/stream?token=${getToken()}`} 
                className="bg-transparent text-xs text-on-surface font-mono flex-1 outline-none px-2"
              />
              <button 
                onClick={() => {
                  navigator.clipboard.writeText(`${window.location.origin}/api/files/${shareTarget.id}/stream?token=${getToken()}`);
                  showToast("Share link copied to clipboard!", "success");
                }}
                className="px-3 py-1.5 rounded-lg bg-primary text-on-primary-fixed text-xs font-bold hover:bg-primary-fixed transition-colors flex items-center gap-1 cursor-pointer"
              >
                <span className="material-symbols-outlined text-[14px]">content_copy</span> Copy
              </button>
            </div>
            <div className="flex justify-end">
              <button 
                onClick={() => setShareTarget(null)}
                className="px-5 py-2 rounded-xl text-sm font-semibold bg-surface-container-highest text-on-surface hover:bg-surface-bright transition-colors cursor-pointer"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 8. New Folder Modal */}
      {showNewFolderModal && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="w-full max-w-md rounded-3xl bg-surface-container-high border border-outline-variant/40 p-space-lg shadow-2xl animate-in zoom-in-95">
            <h3 className="font-headline-sm text-headline-sm font-bold text-on-surface mb-2">Create New Folder</h3>
            <p className="text-body-sm text-on-surface-variant mb-4">Enter a folder name for your Telegram cloud storage.</p>
            <input 
              type="text"
              autoFocus
              placeholder="e.g. Invoices 2026"
              value={newFolderName}
              onChange={(e) => setNewFolderName(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter') handleNewFolderSubmit(); if (e.key === 'Escape') setShowNewFolderModal(false); }}
              className="w-full bg-surface-container-low border border-outline-variant/40 rounded-xl px-4 py-2.5 text-on-surface text-sm outline-none focus:border-primary focus:ring-1 focus:ring-primary mb-6"
            />
            <div className="flex items-center justify-end gap-3">
              <button 
                onClick={() => { setNewFolderName(''); setShowNewFolderModal(false); }}
                className="px-4 py-2 rounded-xl text-sm font-semibold text-on-surface-variant hover:bg-surface-container transition-colors cursor-pointer"
              >
                Cancel
              </button>
              <button 
                onClick={handleNewFolderSubmit}
                className="px-5 py-2 rounded-xl text-sm font-semibold bg-primary text-on-primary-fixed hover:bg-primary-fixed transition-colors shadow-md cursor-pointer"
              >
                Create Folder
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 9. Create Video Category Modal */}
      {showCreateCategoryModal && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="w-full max-w-md rounded-3xl bg-surface-container-high border border-outline-variant/40 p-space-lg shadow-2xl animate-in zoom-in-95">
            <h3 className="font-headline-sm text-headline-sm font-bold text-on-surface mb-2">Create Video Category</h3>
            <p className="text-body-sm text-on-surface-variant mb-4">Add a new sliding cinema slider category (e.g. Sci-Fi Classics, Anime, Documentaries).</p>
            <input 
              type="text"
              autoFocus
              placeholder="e.g. Sci-Fi Favorites"
              value={newCategoryName}
              onChange={(e) => setNewCategoryName(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter') handleCreateCategorySubmit(); if (e.key === 'Escape') setShowCreateCategoryModal(false); }}
              className="w-full bg-surface-container-low border border-outline-variant/40 rounded-xl px-4 py-2.5 text-on-surface text-sm outline-none focus:border-primary focus:ring-1 focus:ring-primary mb-6"
            />
            <div className="flex items-center justify-end gap-3">
              <button 
                onClick={() => { setNewCategoryName(''); setShowCreateCategoryModal(false); }}
                className="px-4 py-2 rounded-xl text-sm font-semibold text-on-surface-variant hover:bg-surface-container transition-colors cursor-pointer"
              >
                Cancel
              </button>
              <button 
                onClick={handleCreateCategorySubmit}
                className="px-5 py-2 rounded-xl text-sm font-semibold bg-primary text-on-primary-fixed hover:bg-primary-fixed transition-colors shadow-md cursor-pointer"
              >
                Add Category
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 10. Create Music Album Modal */}
      {showCreateAlbumModal && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="w-full max-w-md rounded-3xl bg-surface-container-high border border-outline-variant/40 p-space-lg shadow-2xl animate-in zoom-in-95">
            <h3 className="font-headline-sm text-headline-sm font-bold text-on-surface mb-2">Create Music Album</h3>
            <p className="text-body-sm text-on-surface-variant mb-4">Add a new album collection to your lossless streaming shelf.</p>
            <div className="space-y-3 mb-6">
              <div>
                <label className="block text-xs font-semibold text-on-surface-variant mb-1">Album Title</label>
                <input 
                  type="text"
                  autoFocus
                  placeholder="e.g. Ambient Horizons Vol. 1"
                  value={newAlbumName}
                  onChange={(e) => setNewAlbumName(e.target.value)}
                  className="w-full bg-surface-container-low border border-outline-variant/40 rounded-xl px-4 py-2.5 text-on-surface text-sm outline-none focus:border-primary focus:ring-1 focus:ring-primary"
                />
              </div>
              <div>
                <label className="block text-xs font-semibold text-on-surface-variant mb-1">Artist Name (Optional)</label>
                <input 
                  type="text"
                  placeholder="e.g. Marconi Union &amp; Friends"
                  value={newAlbumArtist}
                  onChange={(e) => setNewAlbumArtist(e.target.value)}
                  onKeyDown={(e) => { if (e.key === 'Enter') handleCreateAlbumSubmit(); }}
                  className="w-full bg-surface-container-low border border-outline-variant/40 rounded-xl px-4 py-2.5 text-on-surface text-sm outline-none focus:border-primary focus:ring-1 focus:ring-primary"
                />
              </div>
            </div>
            <div className="flex items-center justify-end gap-3">
              <button 
                onClick={() => { setNewAlbumName(''); setNewAlbumArtist(''); setShowCreateAlbumModal(false); }}
                className="px-4 py-2 rounded-xl text-sm font-semibold text-on-surface-variant hover:bg-surface-container transition-colors cursor-pointer"
              >
                Cancel
              </button>
              <button 
                onClick={handleCreateAlbumSubmit}
                className="px-5 py-2 rounded-xl text-sm font-semibold bg-primary text-on-primary-fixed hover:bg-primary-fixed transition-colors shadow-md cursor-pointer"
              >
                Create Album
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 11. Node Settings & Security Modal */}
      {showSettingsModal && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="w-full max-w-lg rounded-3xl bg-surface-container-high border border-outline-variant/40 p-space-lg shadow-2xl animate-in zoom-in-95">
            <div className="flex items-center justify-between mb-4">
              <div className="flex items-center gap-2">
                <span className="material-symbols-outlined text-primary text-2xl">hub</span>
                <h3 className="font-headline-sm text-headline-sm font-bold text-on-surface">TgDrive Space Station Node</h3>
              </div>
              <button onClick={() => setShowSettingsModal(false)} className="w-8 h-8 rounded-full hover:bg-surface-container text-on-surface-variant flex items-center justify-center cursor-pointer">
                <span className="material-symbols-outlined text-[18px]">close</span>
              </button>
            </div>

            <div className="space-y-3 mb-6 bg-surface-container-low p-4 rounded-2xl border border-outline-variant/30 text-sm">
              <div className="flex justify-between py-1.5 border-b border-outline-variant/20 items-center">
                <span className="text-on-surface-variant font-medium">Node Daemon:</span>
                <span className="flex items-center gap-1.5 text-xs text-primary font-bold bg-primary/10 px-2 py-0.5 rounded-full border border-primary/20">
                  <span className="w-1.5 h-1.5 rounded-full bg-primary animate-ping"></span>
                  Online :: Port 8080
                </span>
              </div>
              <div className="flex justify-between py-1.5 border-b border-outline-variant/20 items-center">
                <span className="text-on-surface-variant font-medium">Telegram Bot:</span>
                <span className="text-on-surface font-mono text-xs">@TgDriveCloud_bot (Amrita Sai DC)</span>
              </div>
              <div className="flex justify-between py-1.5 border-b border-outline-variant/20 items-center">
                <span className="text-on-surface-variant font-medium">Channel Shard Target:</span>
                <span className="text-on-surface font-mono text-xs">-1002384729103</span>
              </div>
              <div className="flex justify-between py-1.5 border-b border-outline-variant/20 items-center">
                <span className="text-on-surface-variant font-medium">AES-256 Master Key:</span>
                <span className="text-primary font-mono text-xs font-bold">Configured in Memory (Protected)</span>
              </div>
              <div className="flex justify-between py-1.5 border-b border-outline-variant/20 items-center">
                <span className="text-on-surface-variant font-medium">Auto Warmup Cache:</span>
                <button 
                  onClick={() => setWarmupEnabled(!warmupEnabled)}
                  className={`w-11 h-6 rounded-full transition-colors relative cursor-pointer ${warmupEnabled ? 'bg-primary' : 'bg-surface-container-highest'}`}
                >
                  <span className={`w-4 h-4 rounded-full bg-white absolute top-1 transition-transform ${warmupEnabled ? 'right-1' : 'left-1'}`}></span>
                </button>
              </div>
              <div className="flex justify-between py-1.5 items-center">
                <span className="text-on-surface-variant font-medium">Storage Quota:</span>
                <span className="text-on-surface font-mono text-xs text-primary flex items-center gap-1">
                  <span className="material-symbols-outlined text-[14px]">all_inclusive</span>
                  Unlimited
                </span>
              </div>
            </div>

            <div className="flex items-center justify-between">
              <button 
                onClick={handleLogout}
                className="flex items-center gap-1.5 text-xs text-error font-semibold hover:underline cursor-pointer"
              >
                <span className="material-symbols-outlined text-[16px]">logout</span>
                Disconnect Node &amp; Sign Out
              </button>
              <button 
                onClick={() => setShowSettingsModal(false)}
                className="px-5 py-2 rounded-xl text-sm font-semibold bg-primary text-on-primary-fixed hover:bg-primary-fixed transition-colors cursor-pointer"
              >
                Done
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 12. Movie Info Modal */}
      {showMovieInfoModal && (() => {
        const movie = selectedMovieForInfo || heroMovie;
        const cleanTitle = getCleanMovieTitle(movie.name);
        const is4K = movie.size > 8 * 1024 * 1024 * 1024;
        const is1080p = movie.size > 1.5 * 1024 * 1024 * 1024 && !is4K;
        const resLabel = is4K ? 'TGDRIVE 4K UHD' : is1080p ? 'TGDRIVE 1080p' : 'TGDRIVE HD';
        const isMKV = movie.name.toLowerCase().endsWith('.mkv') || movie.mime_type.includes('matroska');
        const isMP4 = movie.name.toLowerCase().endsWith('.mp4');
        const codecLabel = isMKV ? 'Matroska Video' : isMP4 ? 'MP4 Video' : 'Web Video';
        const audioLabel = isMKV ? 'Multi-channel Audio' : 'Stereo Audio';

        return (
          <div className="fixed inset-0 z-50 bg-black/80 backdrop-blur-md flex items-center justify-center p-4">
            <div className="w-full max-w-2xl rounded-3xl bg-surface-container-high border border-outline-variant/40 overflow-hidden shadow-2xl animate-in zoom-in-95">
              <div className="relative h-64 w-full bg-surface-container-lowest overflow-hidden">
                <VideoThumbnail 
                  file={movie} 
                  isHero={true} 
                  className="absolute inset-0 w-full h-full object-cover object-center z-0" 
                />
                <div className="absolute inset-0 bg-gradient-to-t from-surface-container-high via-surface-container-high/40 to-transparent z-10" />
                <button 
                  onClick={() => setShowMovieInfoModal(false)}
                  className="absolute top-4 right-4 w-9 h-9 rounded-full bg-black/60 text-white flex items-center justify-center hover:bg-black/80 cursor-pointer z-20 transition-colors"
                >
                  <span className="material-symbols-outlined text-[18px]">close</span>
                </button>
                <div className="absolute bottom-4 left-6 z-20">
                  <span className="px-2 py-0.5 rounded bg-primary/20 text-primary border border-primary/30 text-xs font-bold tracking-widest uppercase">
                    {resLabel}
                  </span>
                  <h2 className="text-4xl font-bold text-white mt-1">{cleanTitle}</h2>
                  <p className="text-xs font-mono text-white/70 mt-0.5">{movie.name}</p>
                </div>
              </div>
              <div className="p-6">
                <div className="flex items-center gap-4 text-xs font-semibold text-on-surface-variant mb-4">
                  <span className="text-primary font-bold">98% Match</span>
                  <span>•</span>
                  <span>{formatFileSize(movie.size)}</span>
                  <span>•</span>
                  <span className="px-1.5 py-0.5 rounded bg-surface-container border border-outline-variant/30 text-on-surface">{codecLabel}</span>
                  <span className="px-1.5 py-0.5 rounded bg-surface-container border border-outline-variant/30 text-on-surface">{audioLabel}</span>
                  <span className="px-1.5 py-0.5 rounded bg-surface-container border border-outline-variant/30 text-primary font-mono">AES-256-GCM</span>
                </div>
                <p className="text-body-md text-on-surface-variant mb-6 leading-relaxed">
                  File: {movie.name}. Decrypted video stream served directly from Telegram Bot storage clusters. Reassembled on-the-fly with multi-part seeking and AES-256-GCM server-side authenticated storage encryption.
                </p>
                <div className="grid grid-cols-2 gap-3 p-3 rounded-2xl bg-surface-container-low text-xs mb-6 border border-outline-variant/30 font-mono">
                  <div>
                    <span className="text-on-surface-variant/70 block">CHUNK CLUSTER</span>
                    <span className="text-on-surface font-semibold">{movie.chunk_count} encrypted parts (16MB)</span>
                  </div>
                  <div>
                    <span className="text-on-surface-variant/70 block">KEY SALT</span>
                    <span className="text-on-surface font-semibold">{movie.encryption_salt ? `${movie.encryption_salt.substring(0, 16)}...` : 'AES-GCM-STANDARD'}</span>
                  </div>
                </div>
                <div className="flex justify-end gap-3">
                  <button 
                    onClick={() => setShowMovieInfoModal(false)}
                    className="px-5 py-2.5 rounded-xl text-sm font-semibold text-on-surface-variant hover:bg-surface-container cursor-pointer"
                  >
                    Close
                  </button>
                  <button 
                    onClick={() => {
                      setShowMovieInfoModal(false);
                      handlePreview(movie);
                    }}
                    className="flex items-center gap-2 px-6 py-2.5 rounded-xl text-sm font-bold bg-primary text-on-primary-fixed hover:bg-primary-fixed shadow-lg cursor-pointer"
                  >
                    <span className="material-symbols-outlined filled text-lg">play_arrow</span> Play Movie
                  </button>
                </div>
              </div>
            </div>
          </div>
        );
      })()}

      {/* Quick Manage Cinema Titles Modal */}
      {showAddCinemaModal && (
        <div 
          className="fixed inset-0 z-50 bg-black/80 backdrop-blur-md flex items-center justify-center p-4 animate-in fade-in duration-200"
          onClick={() => setShowAddCinemaModal(false)}
        >
          <div 
            className="bg-surface-container-high border border-outline-variant/40 rounded-3xl w-full max-w-2xl max-h-[85vh] flex flex-col shadow-2xl overflow-hidden animate-in zoom-in-95 duration-200"
            onClick={(e) => e.stopPropagation()}
          >
            {/* Modal Header */}
            <div className="p-5 border-b border-outline-variant/30 flex items-center justify-between">
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 rounded-2xl bg-primary/10 text-primary flex items-center justify-center">
                  <span className="material-symbols-outlined text-[22px]">theaters</span>
                </div>
                <div>
                  <h3 className="font-bold text-base text-on-surface">Manage Cinema Titles</h3>
                  <p className="text-xs text-on-surface-variant">
                    {cinemaVideos.length} of {allVideoFiles.length} videos featured in Cinema
                  </p>
                </div>
              </div>
              <button 
                onClick={() => setShowAddCinemaModal(false)}
                className="text-on-surface-variant hover:text-on-surface p-1.5 rounded-lg hover:bg-surface-container cursor-pointer transition-colors"
                title="Close"
              >
                <span className="material-symbols-outlined text-[20px]">close</span>
              </button>
            </div>

            {/* Video Search Filter */}
            <div className="p-4 border-b border-outline-variant/20 bg-surface-container-low/50">
              <div className="relative flex items-center">
                <span className="material-symbols-outlined absolute left-3 text-on-surface-variant text-[18px]">search</span>
                <input
                  type="text"
                  value={cinemaModalSearch}
                  onChange={(e) => setCinemaModalSearch(e.target.value)}
                  placeholder="Search video titles in your library..."
                  className="w-full pl-9 pr-4 py-2 rounded-xl bg-surface-container border border-outline-variant/30 text-xs text-on-surface placeholder:text-on-surface-variant/60 focus:outline-none focus:border-primary/60"
                />
              </div>
            </div>

            {/* Video List */}
            <div className="flex-1 overflow-y-auto p-4 space-y-2 custom-scrollbar max-h-[460px]">
              {allVideoFiles.length === 0 ? (
                <div className="py-12 text-center text-on-surface-variant text-xs">
                  <span className="material-symbols-outlined text-3xl mb-2 text-outline">video_file</span>
                  <p>No video files found in your drive.</p>
                </div>
              ) : (
                allVideoFiles
                  .filter(f => !cinemaModalSearch.trim() || f.name.toLowerCase().includes(cinemaModalSearch.toLowerCase().trim()))
                  .map((video) => {
                    const isInCinema = Boolean(video.is_cinema);
                    return (
                      <div 
                        key={video.id}
                        className={`flex items-center justify-between p-3 rounded-2xl border transition-all ${
                          isInCinema 
                            ? 'bg-primary/10 border-primary/40' 
                            : 'bg-surface-container/60 hover:bg-surface-container border-outline-variant/20'
                        }`}
                      >
                        <div className="flex items-center gap-3 min-w-0 flex-1">
                          <div className="w-12 h-10 rounded-xl overflow-hidden bg-surface-container-lowest flex-shrink-0 relative border border-outline-variant/30">
                            <VideoThumbnail file={video} className="w-full h-full object-cover" />
                          </div>
                          <div className="min-w-0 flex-1">
                            <h4 className="text-xs font-bold text-on-surface truncate">{getCleanMovieTitle(video.name)}</h4>
                            <p className="text-[11px] text-on-surface-variant font-mono truncate">
                              {formatFileSize(video.size)} • {video.name}
                            </p>
                          </div>
                        </div>

                        <button
                          onClick={() => handleToggleCinema(video)}
                          className={`ml-3 px-3 py-1.5 rounded-xl text-xs font-semibold flex items-center gap-1.5 transition-all cursor-pointer flex-shrink-0 ${
                            isInCinema 
                              ? 'bg-primary text-on-primary-fixed hover:bg-rose-500 hover:text-white' 
                              : 'bg-surface-container-high hover:bg-primary hover:text-on-primary-fixed text-on-surface border border-outline-variant/30'
                          }`}
                          title={isInCinema ? "Click to remove from Cinema" : "Add to Cinema"}
                        >
                          <span className="material-symbols-outlined text-[16px]">
                            {isInCinema ? 'check_circle' : 'add'}
                          </span>
                          <span>{isInCinema ? 'In Cinema' : 'Add to Cinema'}</span>
                        </button>
                      </div>
                    );
                  })
              )}
            </div>

            {/* Modal Footer */}
            <div className="p-4 border-t border-outline-variant/30 flex items-center justify-between bg-surface-container-low/40">
              <span className="text-xs text-on-surface-variant font-mono">
                {cinemaVideos.length} titles active in Cinema
              </span>
              <button
                onClick={() => setShowAddCinemaModal(false)}
                className="px-5 py-2 rounded-xl text-xs font-bold bg-primary text-on-primary-fixed hover:bg-primary-fixed transition-colors shadow-md cursor-pointer"
              >
                Done
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 13. Interactive Toast Notifications Container */}
      <div className="fixed bottom-24 right-8 z-50 flex flex-col gap-2 pointer-events-none">
        {toasts.map(toast => (
          <div 
            key={toast.id}
            className={`pointer-events-auto flex items-center gap-2.5 px-4 py-3 rounded-2xl shadow-2xl border backdrop-blur-xl animate-in slide-in-from-bottom-2 fade-in text-sm font-medium ${
              toast.type === 'success' 
                ? 'bg-surface-container-high/95 text-on-surface border-primary/40 text-primary' 
                : toast.type === 'error'
                ? 'bg-error-container/95 text-on-error-container border-error/40'
                : 'bg-surface-container-high/95 text-on-surface border-outline-variant/40'
            }`}
          >
            <span className="material-symbols-outlined text-[18px]">{toast.type === 'success' ? 'check_circle' : toast.type === 'error' ? 'error' : 'info'}</span>
            <span>{toast.message}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
