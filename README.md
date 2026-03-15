# 🔍 Chunk Hunter

Scans text documents for predefined phrases ("chunks") and generates a visual report showing where and how often they appear.

## ✨ Features

- 🖥️ **Browser-based GUI** — drag & drop files, view highlighted results instantly
- 📄 **Multiple formats** — supports `.txt`, `.docx`, `.odt`, and `.doc`
- 🟡 **Visual highlighting** — matched chunks are highlighted in yellow in the original text
- 📊 **Statistics** — chunk word count, total words, and chunk frequency percentage
- 📥 **CSV export** — download a detailed breakdown of all matched chunks
- 📦 **Zero installation** — single `.exe` + `chunks.txt`, nothing else needed

## 🚀 Getting started

1. Place `chunks.exe` and `chunks.txt` in the same folder
2. Double-click `chunks.exe`
3. Your browser opens automatically with the Chunk Hunter interface
4. Drag & drop one or more files and click **Scan**

## 📁 Supported file formats

| Format | Extension | Notes |
|--------|-----------|-------|
| Plain text | `.txt` | UTF-8 and Windows-1252 encoding |
| Word | `.docx` | Full support |
| OpenDocument | `.odt` | Full support |
| Legacy Word | `.doc` | Best-effort extraction |

> 💡 For best results with older Word documents, save them as `.docx` first.

## 🗃️ The chunks database

`chunks.txt` is a plain text file with one phrase per line. You can edit it freely to add, remove, or modify chunks. Changes take effect the next time you start the application.

## 🛠️ Building from source

Requires [Go](https://go.dev/) 1.21+.

```bash
# Cross-compile for Windows
make build

# Or manually
env GOOS=windows GOARCH=amd64 go build -o chunks.exe .
```

## 📝 License

MIT
