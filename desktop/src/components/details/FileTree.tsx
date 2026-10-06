import { useEffect, useMemo, useRef, useState, type CSSProperties, type ReactNode } from "react"
import {
  BracesIcon,
  ChevronDownIcon,
  FileCodeIcon,
  FileIcon,
  FileTextIcon,
  FolderIcon,
  FolderOpenIcon,
  ImageIcon,
} from "lucide-react"
import { StatusBadge } from "./StatusBadge"
import { cn } from "@/lib/utils"

type Node<T> = { name: string; path: string; file?: T; children: Node<T>[] }

/** buildTree nests paths by directory, joining single-child directories ("src/util"). */
function buildTree<T extends { path: string }>(files: T[]): Node<T>[] {
  const root: Node<T> = { name: "", path: "", children: [] }
  for (const f of files) {
    const parts = f.path.split("/")
    let at = root
    parts.forEach((part, i) => {
      const path = parts.slice(0, i + 1).join("/")
      const leaf = i === parts.length - 1
      let child = at.children.find((c) => c.name === part && (c.file !== undefined) === leaf)
      if (!child) {
        child = { name: part, path, file: leaf ? f : undefined, children: [] }
        at.children.push(child)
      }
      at = child
    })
  }
  const squash = (n: Node<T>): Node<T> => {
    let out = n
    while (!out.file && out.children.length === 1 && !out.children[0].file) {
      const c = out.children[0]
      out = { ...c, name: `${out.name}/${c.name}` }
    }
    return { ...out, children: out.children.map(squash) }
  }
  return root.children.map(squash)
}

/** Indent per level, as the ReUI tree's --tree-indent; the guide line sits under the chevron. */
const indent = 12
const rowPad = 6

type PathTreeProps<T> = {
  files: T[]
  /** renderFile draws a file's row; name is its last path segment, pad its indent. */
  renderFile: (file: T, name: string, pad: CSSProperties) => ReactNode
  className?: string
}

/**
 * PathTree lists files nested by directory; a click on a directory folds it
 * (internal/ui/filetree.go). The diff's file tree and the Unstaged panel share it.
 * It is drawn like the ReUI tree, but the rows stay plain buttons so they can be
 * drag sources and context-menu triggers.
 */
export function PathTree<T extends { path: string }>({ files, renderFile, className }: PathTreeProps<T>) {
  const tree = useMemo(() => buildTree(files), [files])
  const [folded, setFolded] = useState<Record<string, boolean>>({})

  const render = (nodes: Node<T>[], depth: number): ReactNode =>
    nodes.map((n) => {
      const pad = { paddingLeft: `${depth * indent + rowPad}px` }
      if (n.file) return <li key={`f:${n.path}`}>{renderFile(n.file, n.name, pad)}</li>
      const isFolded = folded[n.path]
      return (
        <li key={`d:${n.path}`}>
          <button
            type="button"
            className="flex h-6 w-full items-center gap-1 rounded-md pr-2 text-left text-xs text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
            style={pad}
            onClick={() => setFolded((f) => ({ ...f, [n.path]: !f[n.path] }))}
            aria-expanded={!isFolded}
          >
            <ChevronDownIcon
              className={cn("size-3.5 shrink-0 opacity-70 transition-transform", isFolded && "-rotate-90")}
              aria-hidden="true"
            />
            {isFolded ? (
              <FolderIcon className="size-3.5 shrink-0 text-primary/70" aria-hidden="true" />
            ) : (
              <FolderOpenIcon className="size-3.5 shrink-0 text-primary/70" aria-hidden="true" />
            )}
            <span className="truncate">
              {n.name}
              <span className="opacity-50">/</span>
            </span>
          </button>
          {isFolded ? null : (
            <ul
              className="relative before:pointer-events-none before:absolute before:inset-y-0 before:left-(--tree-guide) before:w-px before:bg-border"
              style={{ "--tree-guide": `${depth * indent + rowPad + 6}px` } as CSSProperties}
            >
              {render(n.children, depth + 1)}
            </ul>
          )}
        </li>
      )
    })

  return <ul className={cn("flex flex-col gap-px", className)}>{render(tree, 0)}</ul>
}

/** treeFileClass is the look of a file row in a PathTree. */
export const treeFileClass =
  "flex h-6 w-full items-center gap-1.5 rounded-md pr-1.5 text-left font-mono text-xs transition-colors"

/** treeRowSelected and treeRowHover are a row's selected and hover backgrounds, shared by both trees. */
export const treeRowSelected = "bg-foreground/[0.07] font-medium text-foreground dark:bg-foreground/10"
export const treeRowHover = "hover:bg-foreground/[0.04] dark:hover:bg-foreground/[0.06]"

const codeExt = /\.(go|ts|tsx|js|jsx|mjs|cjs|rs|py|rb|java|kt|swift|c|h|cc|cpp|cs|sh|css|scss|html|vue|svelte|sql)$/i
const dataExt = /\.(json|ya?ml|toml|mod|sum|lock|xml)$/i
const textExt = /\.(md|mdx|txt|rst|adoc)$/i
const imageExt = /\.(png|jpe?g|gif|svg|webp|ico|bmp)$/i

/** FileTypeIcon picks a glyph by extension, as the ReUI file explorer tree does. */
function FileTypeIcon({ name }: { name: string }) {
  const props = { className: "size-3.5 shrink-0 text-muted-foreground", "aria-hidden": true } as const
  if (codeExt.test(name)) return <FileCodeIcon {...props} />
  if (dataExt.test(name)) return <BracesIcon {...props} />
  if (textExt.test(name)) return <FileTextIcon {...props} />
  if (imageExt.test(name)) return <ImageIcon {...props} />
  return <FileIcon {...props} />
}

/** TreeFileLabel is the icon, name and status letter of a file row. */
export function TreeFileLabel({ name, status }: { name: string; status?: string }) {
  return (
    <>
      <FileTypeIcon name={name} />
      <span className="min-w-0 flex-1 truncate">{name}</span>
      {status ? <StatusBadge status={status} /> : null}
    </>
  )
}

type Props = {
  files: { path: string; status: string }[]
  /** The file the diff is at. */
  active: string
  onSelect: (path: string) => void
}

/** FileTree lists the files of the diff shown full screen, next to it, to jump between them. */
export function FileTree({ files, active, onSelect }: Props) {
  const activeRef = useRef<HTMLButtonElement | null>(null)

  useEffect(() => {
    activeRef.current?.scrollIntoView({ block: "nearest" })
  }, [active])

  return (
    // select-none: a click or shift-click on a row must not paint a native text selection
    // over the file names, which WebKit does for button text.
    <div className="flex h-full min-h-0 flex-col border-r bg-sidebar select-none" data-testid="file-tree">
      <p className="flex h-8 shrink-0 items-center border-b px-3 text-xs font-medium text-muted-foreground">
        {files.length} file{files.length === 1 ? "" : "s"}
      </p>
      <PathTree
        files={files}
        className="min-h-0 flex-1 overflow-auto p-1.5"
        renderFile={(f, name, pad) => {
          const isActive = f.path === active
          return (
            <button
              type="button"
              ref={isActive ? activeRef : undefined}
              data-testid="tree-file"
              className={cn(treeFileClass, isActive ? treeRowSelected : treeRowHover)}
              style={pad}
              onClick={() => onSelect(f.path)}
              title={f.path}
            >
              <TreeFileLabel name={name} status={f.status} />
            </button>
          )
        }}
      />
    </div>
  )
}
