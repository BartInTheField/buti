import { useEffect, type ReactNode } from "react"
import { usePanelRef } from "react-resizable-panels"
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable"
import { setDetailsView, useDetailsView } from "./store"

/**
 * DetailsLayout puts the details pane under the workspace, hides it (d) or gives it the
 * window (D) by collapsing a panel rather than unmounting it, so the lanes keep their
 * scroll and the diff its cursor. Dragging a panel shut does the same as the keys.
 */
export function DetailsLayout({ top, details }: { top: ReactNode; details: ReactNode }) {
  const view = useDetailsView()
  const topRef = usePanelRef()
  const detailsRef = usePanelRef()

  useEffect(() => {
    const t = topRef.current
    const d = detailsRef.current
    if (!t || !d) return
    if (view.full) {
      d.expand()
      t.collapse()
    } else if (!view.visible) {
      t.expand()
      d.collapse()
    } else {
      t.expand()
      d.expand()
    }
  }, [view.full, view.visible, topRef, detailsRef])

  return (
    <ResizablePanelGroup orientation="vertical" className="min-h-0 flex-1">
      <ResizablePanel
        panelRef={topRef}
        defaultSize="58"
        minSize="30"
        collapsible
        onResize={() => {
          const collapsed = topRef.current?.isCollapsed() ?? false
          if (collapsed !== view.full) setDetailsView({ full: collapsed })
        }}
      >
        {top}
      </ResizablePanel>
      <ResizableHandle withHandle />
      <ResizablePanel
        panelRef={detailsRef}
        defaultSize="42"
        minSize="20"
        collapsible
        onResize={() => {
          const collapsed = detailsRef.current?.isCollapsed() ?? false
          if (collapsed === view.visible && !view.full) setDetailsView({ visible: !collapsed })
        }}
      >
        {details}
      </ResizablePanel>
    </ResizablePanelGroup>
  )
}
