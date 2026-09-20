import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { FileItem, FolderItem, FileCategory, getFileCategory, formatFileSize } from '../types';
import { listFolder, createFolder, deleteFolder, deleteFile, clearToken } from '../api/client';
import FileCard from './FileCard';
import UploadModal from './UploadModal';
import FilePreview from './FilePreview';
import {
  ShieldCheck,
  Search,
  UploadCloud,
  FolderPlus,
  Folder,
  LayoutGrid,
  List,
  ArrowUpDown,
  LogOut,
  ChevronRight,
  HardDrive,
  Lock,
  Layers,
  Sparkles,
  Trash2,
  Film,
  FileText,
  FileCode,
  FileArchive,
  RefreshCw,
  Plus,
  X,
  Menu,
} from 'lucide-react';

interface BreadcrumbItem {
  id: string | null;
  name: string;
}

export default function Layout() {
  const [folders, setFolders] = useState<FolderItem[]>([]);
  const [files, setFiles] = useState<FileItem[]>([]);
  const [currentFolder, setCurrentFolder] = useState<FolderItem | null>(null);
  const [breadcrumbs, setBreadcrumbs] = useState<BreadcrumbItem[]>([{ id: null, name: 'Personal Vault' }]);
  const [showUpload, setShowUpload] = useState(false);
  const [previewFile, setPreviewFile] = useState<FileItem | null>(null);
  const [loading, setLoading] = useState(true);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [newFolderName, setNewFolderName] = useState('');
  const [showNewFolder, setShowNewFolder] = useState(false);

  // Search & Filter & View state
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedCategory, setSelectedCategory] = useState<FileCategory>('all');
  const [viewMode, setViewMode] = useState<'grid' | 'list'>('grid');
  const [sortBy, setSortBy] = useState<'date' | 'name' | 'size'>('date');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');
  const [toastMessage, setToastMessage] = useState<string | null>(null);

  const showToast = (msg: string) => {
    setToastMessage(msg);
    setTimeout(() => setToastMessage(null), 3500);
  };

  const loadFolder = useCallback(async (folderId?: string) => {
    setLoading(true);
    try {
      const data = await listFolder(folderId);
      setFolders(data.folders || []);
      setFiles(data.files || []);
      setCurrentFolder(data.folder || null);
    } catch (err) {
      console.error('Failed to load folder:', err);
      showToast('Error syncing with Telegram storage');
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    loadFolder();
  }, [loadFolder]);

  // Keyboard shortcut Ctrl+K for search
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
        e.preventDefault();
        document.getElementById('global-search-input')?.focus();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  const navigateToFolder = (folder: FolderItem) => {
    setBreadcrumbs((prev) => [...prev, { id: folder.id, name: folder.name }]);
    loadFolder(folder.id);
  };

  const navigateToBreadcrumb = (index: number) => {
    const crumb = breadcrumbs[index];
    setBreadcrumbs(breadcrumbs.slice(0, index + 1));
    loadFolder(crumb.id || undefined);
  };

  const handleCreateFolder = async () => {
    if (!newFolderName.trim()) return;
    try {
      await createFolder(newFolderName.trim(), currentFolder?.id);
      setNewFolderName('');
      setShowNewFolder(false);
      showToast(`Folder "${newFolderName.trim()}" created`);
      loadFolder(currentFolder?.id);
    } catch (err) {
      console.error('Failed to create folder:', err);
      showToast('Failed to create folder');
    }
  };

  const handleDeleteFolder = async (id: string, name: string) => {
    if (!confirm(`Delete folder "${name}" and all encrypted files within?`)) return;
    try {
      await deleteFolder(id);
      showToast(`Folder "${name}" deleted`);
      loadFolder(currentFolder?.id);
    } catch (err) {
      console.error('Failed to delete folder:', err);
      showToast('Failed to delete folder');
    }
  };

  const handleDeleteFile = async (id: string) => {
    try {
      await deleteFile(id);
      showToast('File deleted from Telegram channel');
      loadFolder(currentFolder?.id);
    } catch (err) {
      console.error('Failed to delete file:', err);
      showToast('Failed to delete file');
    }
  };

  const handleLogout = () => {
    clearToken();
    window.location.reload();
  };

  // Filter and sort files
  const filteredFiles = useMemo(() => {
    return files
      .filter((file) => {
        const matchesSearch = file.name.toLowerCase().includes(searchQuery.toLowerCase());
        if (!matchesSearch) return false;
        if (selectedCategory === 'all') return true;
        return getFileCategory(file.mime_type, file.name) === selectedCategory;
      })
      .sort((a, b) => {
        let result = 0;
        if (sortBy === 'name') result = a.name.localeCompare(b.name);
        else if (sortBy === 'size') result = a.size - b.size;
        else if (sortBy === 'date') result = new Date(a.created_at).getTime() - new Date(b.created_at).getTime();
        return sortOrder === 'desc' ? -result : result;
      });
  }, [files, searchQuery, selectedCategory, sortBy, sortOrder]);

  const totalBytes = useMemo(() => files.reduce((acc, f) => acc + f.size, 0), [files]);
  const totalChunks = useMemo(() => files.reduce((acc, f) => acc + f.chunk_count, 0), [files]);

  return (
    <div className="vault-app-shell">
      {/* Toast Notification */}
      {toastMessage && (
        <div className="vault-toast-notification">
          <Sparkles size={16} className="text-accent" />
          <span>{toastMessage}</span>
        </div>
      )}

      {/* Top Navigation Bar */}
      <header className="vault-navbar">
        <div className="nav-left-section">
          <button
            className="mobile-nav-toggle"
            onClick={() => setSidebarOpen(!sidebarOpen)}
            aria-label="Toggle Navigation"
          >
            <Menu size={22} />
          </button>
          <div className="vault-brand" onClick={() => navigateToBreadcrumb(0)}>
            <div className="brand-shield-icon">
              <ShieldCheck size={22} />
            </div>
            <span className="brand-name">TgDrive</span>
            <span className="brand-version-badge">PRO</span>
          </div>
        </div>

        {/* Global Search Bar */}
        <div className="nav-search-section">
          <div className="search-bar-glass">
            <Search size={16} className="search-icon-prefix" />
            <input
              id="global-search-input"
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search encrypted files..."
              className="search-input-field"
            />
            {searchQuery ? (
              <button className="search-clear-btn" onClick={() => setSearchQuery('')}>
                <X size={14} />
              </button>
            ) : (
              <span className="search-shortcut-hint">Ctrl K</span>
            )}
          </div>
        </div>

        {/* Right Status & Controls */}
        <div className="nav-right-section">
          <div className="telegram-status-pill">
            <span className="pulsing-green-dot"></span>
            <span className="pill-text">Telegram Cloud Connected</span>
          </div>
          <button className="btn-icon-logout" onClick={handleLogout} title="Sign Out">
            <LogOut size={18} />
          </button>
        </div>
      </header>

      <div className="vault-body-layout">
        {/* Left Sidebar */}
        <aside className={`vault-sidebar ${sidebarOpen ? 'sidebar-visible' : ''}`}>
          {/* Action Buttons */}
          <div className="sidebar-actions-wrap">
            <button
              id="upload-trigger-btn"
              className="btn-upload-primary"
              onClick={() => { setShowUpload(true); setSidebarOpen(false); }}
            >
              <UploadCloud size={18} />
              <span>Upload & Encrypt</span>
            </button>
            <button
              className="btn-folder-secondary"
              onClick={() => setShowNewFolder(true)}
              title="Create new folder"
            >
              <FolderPlus size={18} />
            </button>
          </div>

          {/* Navigation Category Filter */}
          <div className="sidebar-section-title">CATEGORIES</div>
          <nav className="sidebar-categories-nav">
            <button
              className={`cat-nav-item ${selectedCategory === 'all' ? 'cat-active' : ''}`}
              onClick={() => { setSelectedCategory('all'); setSidebarOpen(false); }}
            >
              <Layers size={17} />
              <span>All Files</span>
              <span className="cat-count">{files.length}</span>
            </button>
            <button
              className={`cat-nav-item ${selectedCategory === 'media' ? 'cat-active' : ''}`}
              onClick={() => { setSelectedCategory('media'); setSidebarOpen(false); }}
            >
              <Film size={17} />
              <span>Videos & Photos</span>
            </button>
            <button
              className={`cat-nav-item ${selectedCategory === 'docs' ? 'cat-active' : ''}`}
              onClick={() => { setSelectedCategory('docs'); setSidebarOpen(false); }}
            >
              <FileText size={17} />
              <span>Documents & PDFs</span>
            </button>
            <button
              className={`cat-nav-item ${selectedCategory === 'code' ? 'cat-active' : ''}`}
              onClick={() => { setSelectedCategory('code'); setSidebarOpen(false); }}
            >
              <FileCode size={17} />
              <span>Code & Scripts</span>
            </button>
            <button
              className={`cat-nav-item ${selectedCategory === 'archives' ? 'cat-active' : ''}`}
              onClick={() => { setSelectedCategory('archives'); setSidebarOpen(false); }}
            >
              <FileArchive size={17} />
              <span>Archives & Zips</span>
            </button>
          </nav>

          {/* Storage Telemetry Card */}
          <div className="sidebar-telemetry-card">
            <div className="telemetry-header">
              <HardDrive size={16} className="text-accent" />
              <span>Cloud Telemetry</span>
            </div>
            <div className="telemetry-stats-grid">
              <div className="stat-box">
                <span className="stat-label">Stored</span>
                <span className="stat-val">{formatFileSize(totalBytes)}</span>
              </div>
              <div className="stat-box">
                <span className="stat-label">Chunks</span>
                <span className="stat-val">{totalChunks}</span>
              </div>
            </div>
            <div className="telemetry-footer-lock">
              <Lock size={12} className="text-emerald" />
              <span>AES-256-GCM Hardware Cipher</span>
            </div>
          </div>
        </aside>

        {/* Main Content Area */}
        <main className="vault-main-viewport">
          {/* Breadcrumbs & View Options Bar */}
          <div className="viewport-controls-bar">
            {/* Breadcrumb Path */}
            <div className="breadcrumbs-chain">
              {breadcrumbs.map((crumb, idx) => (
                <React.Fragment key={crumb.id || 'root'}>
                  {idx > 0 && <ChevronRight size={14} className="breadcrumb-separator" />}
                  <button
                    className={`breadcrumb-node ${idx === breadcrumbs.length - 1 ? 'node-active' : ''}`}
                    onClick={() => navigateToBreadcrumb(idx)}
                  >
                    {idx === 0 && <HardDrive size={14} className="root-drive-icon" />}
                    <span>{crumb.name}</span>
                  </button>
                </React.Fragment>
              ))}
            </div>

            {/* View and Sort Controls */}
            <div className="view-sort-controls">
              {/* Sort selector */}
              <div className="sort-dropdown-wrap">
                <ArrowUpDown size={14} className="sort-icon-inline" />
                <select
                  className="sort-dropdown"
                  value={sortBy}
                  onChange={(e) => setSortBy(e.target.value as any)}
                >
                  <option value="date">Date Uploaded</option>
                  <option value="name">File Name</option>
                  <option value="size">File Size</option>
                </select>
                <button
                  className="sort-order-toggle"
                  onClick={() => setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc')}
                  title={`Order: ${sortOrder === 'asc' ? 'Ascending' : 'Descending'}`}
                >
                  {sortOrder === 'asc' ? '↑' : '↓'}
                </button>
              </div>

              {/* View Switcher: Grid vs List */}
              <div className="view-mode-toggle">
                <button
                  className={`mode-btn ${viewMode === 'grid' ? 'mode-active' : ''}`}
                  onClick={() => setViewMode('grid')}
                  title="Grid View"
                >
                  <LayoutGrid size={16} />
                </button>
                <button
                  className={`mode-btn ${viewMode === 'list' ? 'mode-active' : ''}`}
                  onClick={() => setViewMode('list')}
                  title="List View"
                >
                  <List size={16} />
                </button>
              </div>

              <button
                className="refresh-sync-btn"
                onClick={() => loadFolder(currentFolder?.id)}
                title="Refresh Folder"
              >
                <RefreshCw size={15} className={loading ? 'animate-spin' : ''} />
              </button>
            </div>
          </div>

          {/* New Folder Inline Form */}
          {showNewFolder && (
            <div className="new-folder-dialog animate-fade-in">
              <FolderPlus size={20} className="text-accent" />
              <input
                type="text"
                value={newFolderName}
                onChange={(e) => setNewFolderName(e.target.value)}
                placeholder="New Folder Name..."
                className="new-folder-input"
                autoFocus
                onKeyDown={(e) => e.key === 'Enter' && handleCreateFolder()}
              />
              <button className="btn-primary-sm" onClick={handleCreateFolder}>
                Create
              </button>
              <button className="btn-ghost-sm" onClick={() => setShowNewFolder(false)}>
                Cancel
              </button>
            </div>
          )}

          {/* Content Loading State */}
          {loading && (
            <div className="viewport-loading-state">
              <div className="vault-spinner"></div>
              <p>Synchronizing encrypted storage...</p>
            </div>
          )}

          {/* Folders Section */}
          {!loading && folders.length > 0 && (
            <section className="folders-section">
              <h4 className="section-label">FOLDERS</h4>
              <div className="folders-grid">
                {folders.map((folder) => (
                  <div
                    key={folder.id}
                    className="folder-card-glass"
                    onClick={() => navigateToFolder(folder)}
                  >
                    <div className="folder-card-left">
                      <div className="folder-icon-glow">
                        <Folder size={22} className="folder-svg" />
                      </div>
                      <span className="folder-name-text" title={folder.name}>{folder.name}</span>
                    </div>
                    <button
                      className="folder-delete-btn"
                      onClick={(e) => {
                        e.stopPropagation();
                        handleDeleteFolder(folder.id, folder.name);
                      }}
                      title="Delete Folder"
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                ))}
              </div>
            </section>
          )}

          {/* Files Section */}
          {!loading && (
            <section className="files-section">
              {folders.length > 0 && <h4 className="section-label">FILES</h4>}

              {filteredFiles.length > 0 ? (
                <div className={viewMode === 'grid' ? 'files-grid-cards' : 'files-list-rows'}>
                  {filteredFiles.map((file) => (
                    <FileCard
                      key={file.id}
                      file={file}
                      viewMode={viewMode}
                      onDelete={handleDeleteFile}
                      onPreview={(f) => setPreviewFile(f)}
                    />
                  ))}
                </div>
              ) : (
                <div className="empty-vault-state">
                  <div className="empty-vault-icon">
                    <ShieldCheck size={56} className="empty-shield-svg" />
                  </div>
                  <h3>Your Secure Vault is Empty</h3>
                  <p>Drop your files, movies, and documents here to stream and store encrypted in Telegram.</p>
                  <button className="btn-empty-upload" onClick={() => setShowUpload(true)}>
                    <UploadCloud size={18} />
                    <span>Upload First File</span>
                  </button>
                </div>
              )}
            </section>
          )}
        </main>
      </div>

      {/* Upload Modal */}
      {showUpload && (
        <UploadModal
          folderId={currentFolder?.id}
          onClose={() => setShowUpload(false)}
          onUploadComplete={() => {
            setShowUpload(false);
            showToast('Files successfully encrypted & uploaded to Telegram');
            loadFolder(currentFolder?.id);
          }}
        />
      )}

      {/* File Preview Modal */}
      {previewFile && (
        <FilePreview
          file={previewFile}
          onClose={() => setPreviewFile(null)}
        />
      )}
    </div>
  );
}
