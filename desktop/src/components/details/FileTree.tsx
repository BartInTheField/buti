import { useEffect, useMemo, useRef, useState } from "react"
import { ChevronDownIcon, ChevronRightIcon, FileIcon } from "lucide-react"
import { cn } from "@/lib/utils"

type Node = { name: string; path: string; dir: boolean; status?: string; children: Node[] }

/** buildTree nests paths by directory, joining single-child directories ("src/util"). */
function buildTree(files: { path: string; status: string }[]): Node[] {
  const root: Node = { name: "", path: "", dir: true, children: [] }
  for (const f of files) {
    const parts = f.path.split("/")
    let at = root
    parts.forEach((part, i) => {
      const path = parts.slice(0, i + 1).join("/")
      const leaf = i === parts.length - 1
      let child = at.children.find((c) => c.name === part && c.dir === !leaf)
      if (!child) {
        child = { name: part, path, dir: !leaf, status: leaf ? f.status : undefined, children: [] }
        at.children.push(child)
      }
      at = child
    })
  }
  const squash = (n: Node): Node => {
    let out = n
    while (out.dir && out.children.length === 1 && out.children[0].dir) {
      const c = out.children[0]
      out = { ...c, name: `${out.name}/${c.name}` }
    }
    return { ...out, children: out.children.map(squash) }
  }
  return root.children.map(squash)
}

type Props = {
  files: { path: string; status: string }[]
  /** The file the diff is at. */
  active: string
  onSelect: (path: string) => void
}

/**
 * FileTree lists the files of the diff shown full screen, next to it, to jump between
 * them; a click on a directory folds it (internal/ui/filetree.go).
 */
export function FileTree({ files, active, onSelect }: Props) {
  const tree = useMemo(() => buildTree(files), [files])
  const [folded, setFolded] = useState<Record<string, boolean>>({})
  const activeRef = useRef<HTMLButtonElement | null>(null)

  useEffect(() => {
    activeRef.current?.scrollIntoView({ block: "nearest" })
  }, [active])

  const render = (nodes: Node[], depth: number) =>
    nodes.map((n) => {
      const pad = { paddingLeft: `${depth * 12 + 8}px` }
      if (n.dir) {
        const isFolded = folded[n.path]
        return (
          <li key={`d:${n.path}`}>
            <button
              type="button"
              className="flex h-6 w-full items-center gap-1 rounded-sm pr-2 text-left text-xs text-muted-foreground hover:bg-accent"
              style={pad}
              onClick={() => setFolded((f) => ({ ...f, [n.path]: !f[n.path] }))}
              aria-expanded={!isFolded}
            >
              {isFolded ? <ChevronRightIcon className="size-3 shrink-0" /> : <ChevronDownIcon className="size-3 shrink-0" />}
              <span className="truncate">{n.name}/</span>
            </button>
            {isFolded ? null : <ul>{render(n.children, depth + 1)}</ul>}
          </li>
        )
      }
      const isActive = n.path === active
      return (
        <li key={`f:${n.path}`}>
          <button
            type="button"
            ref={isActive ? activeRef : undefined}
            data-testid="tree-file"
            className={cn(
              "flex h-6 w-full items-center gap-1.5 rounded-sm pr-2 text-left font-mono text-xs hover:bg-accent",
              isActive && "bg-accent font-medium",
            )}
            style={pad}
            onClick={() => onSelect(n.path)}
            title={n.path}
          >
            <FileIcon className="size-3 shrink-0 opacity-60" />
            <span className="min-w-0 flex-1 truncate">{n.name}</span>
            <span className="text-[10px] text-muted-foreground uppercase">{n.status?.slice(0, 1)}</span>
          </button>
        </li>
      )
    })

  return (
    <div className="flex h-full min-h-0 flex-col border-r bg-sidebar" data-testid="file-tree">
      <p className="border-b px-3 py-1.5 text-xs font-medium text-muted-foreground">
        {files.length} file{files.length === 1 ? "" : "s"}
      </p>
      <ul className="min-h-0 flex-1 overflow-auto p-1">{render(tree, 0)}</ul>
    </div>
  )
}
