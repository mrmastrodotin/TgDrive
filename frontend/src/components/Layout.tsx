import React, { useState, useEffect, useCallback } from 'react';
import { FileItem, FolderItem } from '../types';
import { listFolder, createFolder, deleteFolder, deleteFile, clearToken } from '../api/client';
import FileCard from './FileCard';
import UploadModal from './UploadModal';
import FilePreview from './FilePreview';

interface BreadcrumbItem {
  id: string | null;
  name: string;
}

export default function Layout() {
  const [folders, setFolders] = useState<FolderItem[]>([]);
  const [files, setFiles] = useState<FileItem[]>([]);
  const [currentFolder, setCurrentFolder] = useState<FolderItem | null>(null);
  const [breadcrumbs, setBreadcrumbs] = useState<BreadcrumbItem[]>([{ id: null, name: 'My Drive' }]);
  const [showUpload, setShowUpload] = useState(false);
  const [previewFile, setPreviewFile] = useState<FileItem | null>(null);
  const [loading, setLoading] = useState(true);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [newFolderName, setNewFolderName] = useState('');
  const [showNewFolder, setShowNewFolder] = useState(false);

  const loadFolder = useCallback(async (folderId?: string) => {
    setLoading(true);
    try {
      const data = await listFolder(folderId);
      setFolders(data.folders || []);
      setFiles(data.files || []);
      setCurrentFolder(data.folder || null);
    } catch (err) {
      console.error('Failed to load folder:', err);
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    loadFolder();
  }, [loadFolder]);

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
      loadFolder(currentFolder?.id);
    } catch (err) {
      console.error('Failed to create folder:', err);
    }
  };

  const handleDeleteFolder = async (id: string) => {
    if (!confirm('Delete this folder and all its contents?')) return;
    try {
      await deleteFolder(id);
      loadFolder(currentFolder?.id);
    } catch (err) {
      console.error('Failed to delete folder:', err);
    }
  };

  const handleDeleteFile = async (id: string) => {
    try {
      await deleteFile(id);
      loadFolder(currentFolder?.id);
    } catch (err) {
      console.error('Failed to delete file:', err);
    }
  };

  const handleLogout = () => {
    clearToken();
    window.location.reload();
  };

  return (
    <div className="app-shell">
      {/* Header */}
      <header className="app-header">
        <div className="header-left">
          <button
            id="sidebar-toggle"
            className="btn-icon sidebar-toggle"
            onClick={() => setSidebarOpen(!sidebarOpen)}
          >
            ☰
          </button>
          <span className="app-logo">⚡ TgDrive</span>
        </div>
        <div className="header-right">
          <div className="encryption-badge">
            <span className="badge-dot"></span>
            AES-256 Active
          </div>
          <button id="logout-btn" className="btn-ghost" onClick={handleLogout}>
            Logout
          </button>
        </div>
      </header>

      <div className="app-body">
        {/* Sidebar */}
        <aside className={`app-sidebar ${sidebarOpen ? 'sidebar-open' : ''}`}>
          <nav className="sidebar-nav">
            <button
              id="upload-btn"
              className="btn-primary sidebar-upload"
              onClick={() => { setShowUpload(true); setSidebarOpen(false); }}
            >
              ＋ Upload File
            </button>

            <div className="sidebar-section-label">Storage</div>
            <button
              className="sidebar-item sidebar-item-active"
              onClick={() => { navigateToBreadcrumb(0); setSidebarOpen(false); }}
            >
              📁 My Drive
            </button>
          </nav>

          <div className="sidebar-footer">
            <div className="sidebar-stat">
              <span>{files.length} files</span>
              <span>{folders.length} folders</span>
            </div>
          </div>
        </aside>

        {/* Overlay for mobile sidebar */}
        {sidebarOpen && (
          <div className="sidebar-overlay" onClick={() => setSidebarOpen(false)} />
        )}

        {/* Main Content */}
        <main className="app-main">
          {/* Breadcrumbs */}
          <div className="breadcrumbs">
            {breadcrumbs.map((crumb, idx) => (
              <span key={idx} className="breadcrumb-item">
                {idx > 0 && <span className="breadcrumb-sep">/</span>}
                <button
                  className={`breadcrumb-link ${idx === breadcrumbs.length - 1 ? 'breadcrumb-current' : ''}`}
                  onClick={() => navigateToBreadcrumb(idx)}
                >
                  {crumb.name}
                </button>
              </span>
            ))}
          </div>

          {/* Toolbar */}
          <div className="toolbar">
            <button
              id="new-folder-btn"
              className="btn-secondary"
              onClick={() => setShowNewFolder(!showNewFolder)}
            >
              📁 New Folder
            </button>
            <button
              id="mobile-upload-btn"
              className="btn-primary mobile-only"
              onClick={() => setShowUpload(true)}
            >
              ＋ Upload
            </button>
          </div>

          {/* New Folder Input */}
          {showNewFolder && (
            <div className="new-folder-form">
              <input
                id="new-folder-input"
                type="text"
                value={newFolderName}
                onChange={(e) => setNewFolderName(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleCreateFolder()}
                placeholder="Folder name"
                className="new-folder-input"
                autoFocus
              />
              <button className="btn-primary" onClick={handleCreateFolder}>Create</button>
              <button className="btn-ghost" onClick={() => setShowNewFolder(false)}>Cancel</button>
            </div>
          )}

          {/* Loading State */}
          {loading && (
            <div className="loading-state">
              <div className="spinner"></div>
              <p>Decrypting contents...</p>
            </div>
          )}

          {/* Empty State */}
          {!loading && folders.length === 0 && files.length === 0 && (
            <div className="empty-state">
              <div className="empty-icon">📂</div>
              <p className="empty-text">This folder is empty</p>
              <p className="empty-hint">Upload files or create a folder to get started</p>
            </div>
          )}

          {/* Folders Grid */}
          {folders.length > 0 && (
            <div className="section">
              <h3 className="section-title">Folders</h3>
              <div className="folder-grid">
                {folders.map((folder) => (
                  <div
                    key={folder.id}
                    className="folder-card"
                    onClick={() => navigateToFolder(folder)}
                    role="button"
                    tabIndex={0}
                  >
                    <span className="folder-card-icon">📁</span>
                    <span className="folder-card-name">{folder.name}</span>
                    <button
                      className="btn-icon btn-danger folder-delete"
                      onClick={(e) => {
                        e.stopPropagation();
                        handleDeleteFolder(folder.id);
                      }}
                      title="Delete folder"
                    >
                      🗑️
                    </button>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Files Grid */}
          {files.length > 0 && (
            <div className="section">
              <h3 className="section-title">Files</h3>
              <div className="file-grid">
                {files.map((file) => (
                  <FileCard
                    key={file.id}
                    file={file}
                    onDelete={handleDeleteFile}
                    onPreview={setPreviewFile}
                  />
                ))}
              </div>
            </div>
          )}
        </main>
      </div>

      {/* Modals */}
      {showUpload && (
        <UploadModal
          folderId={currentFolder?.id}
          onClose={() => setShowUpload(false)}
          onUploadComplete={() => loadFolder(currentFolder?.id)}
        />
      )}

      {previewFile && (
        <FilePreview
          file={previewFile}
          onClose={() => setPreviewFile(null)}
        />
      )}
    </div>
  );
}
