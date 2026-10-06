import { ListIcon, XIcon } from "lucide-react"
import { Badge } from "@/components/reui/badge"
import { Button } from "@/components/ui/button"
import { Kbd } from "@/components/ui/kbd"
import { Separator } from "@/components/ui/separator"
import { Toggle } from "@/components/ui/toggle"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { cn } from "@/lib/utils"
import { describeAll, sourcesFor, verbs, type TargetState, type Verb } from "@/target"
import { useTargetMode, verbInfo } from "./context"
import { cancelTarget, switchVerb, updateTarget, useTargetHover } from "./store"

/** A pressed toggle in the bar must read as on at a glance; muted is too faint. */
const pressedClass = "data-[state=on]:bg-primary data-[state=on]:text-primary-foreground"

/**
 * TargetHintBar floats over the bottom of the window while a verb waits for a target,
 * like the TUI's status bar: what the hovered target would do, the verb's options, and
 * how to pick from a list or cancel. Fixed size, so nothing under it moves.
 */
export function TargetHintBar({ state }: { state: TargetState }) {
  const { decos, openList } = useTargetMode()
  const hover = useTargetHover()
  const plan = hover ? decos.get(hover)?.plan : null
  const what = describeAll(state.sources)
  const info = verbInfo[state.verb]
  const sided = state.verb !== "squash" && !state.newBranchHere

  return (
    <div
      role="toolbar"
      aria-label="Choose a target"
      data-testid="target-bar"
      className="fixed bottom-4 left-1/2 z-40 flex w-[min(56rem,calc(100vw-2rem))] -translate-x-1/2 flex-col gap-1 rounded-xl border bg-muted/80 p-1 text-popover-foreground shadow-lg backdrop-blur-sm"
    >
      <div className="flex h-9 items-center gap-2 rounded-lg border bg-popover px-2 shadow-xs">
        <Badge size="lg" className={cn("min-w-16 uppercase tracking-wide", info.chip)}>
          {info.title}
        </Badge>
        <p
          data-testid="target-desc"
          className={cn("min-w-0 flex-1 truncate text-sm", !plan && "text-muted-foreground")}
        >
          {plan ? plan.desc : `Click a target for ${what}`}
        </p>
        <Button size="sm" variant="outline" onClick={openList}>
          <ListIcon />
          Targets
          <Kbd>/</Kbd>
        </Button>
        <Button size="sm" variant="ghost" onClick={cancelTarget}>
          <XIcon />
          Cancel
          <Kbd>Esc</Kbd>
        </Button>
      </div>
      <div className="flex h-8 items-center gap-2 px-1">
        <ToggleGroup
          type="single"
          size="sm"
          spacing={0}
          variant="outline"
          value={state.verb}
          onValueChange={(v) => v && switchVerb(state, v as Verb)}
          aria-label="Verb"
        >
          {verbs.map((v) => (
            <ToggleGroupItem
              key={v}
              value={v}
              disabled={"why" in sourcesFor(v, state.sources)}
              className={cn("gap-1.5", pressedClass)}
            >
              {verbInfo[v].title}
              <Kbd>{verbInfo[v].key}</Kbd>
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <Separator orientation="vertical" className="h-5" />
        {sided ? (
          <ToggleGroup
            type="single"
            size="sm"
            spacing={0}
            variant="outline"
            value={state.side}
            onValueChange={(v) => v && updateTarget({ side: v as TargetState["side"] })}
            aria-label="Above or below a commit"
          >
            <ToggleGroupItem value="above" className={pressedClass}>
              Above
            </ToggleGroupItem>
            <ToggleGroupItem value="below" className={pressedClass}>
              Below
            </ToggleGroupItem>
          </ToggleGroup>
        ) : null}
        {sided ? <Kbd>a</Kbd> : null}
        {state.verb === "commit" || state.verb === "pick" ? (
          <Option
            label="New branch here"
            k="b"
            pressed={state.newBranchHere}
            onChange={(on) => updateTarget({ newBranchHere: on })}
          />
        ) : null}
        {state.verb === "commit" ? (
          <Option
            label="Empty message"
            k="e"
            pressed={state.emptyMsg}
            onChange={(on) => updateTarget({ emptyMsg: on })}
          />
        ) : null}
        {state.verb === "squash" ? (
          <Option
            label="Keep target message"
            k="u"
            pressed={state.useTarget}
            onChange={(on) => updateTarget({ useTarget: on })}
          />
        ) : null}
      </div>
    </div>
  )
}

function Option({
  label,
  k,
  pressed,
  onChange,
}: {
  label: string
  k: string
  pressed: boolean
  onChange: (on: boolean) => void
}) {
  return (
    <Toggle size="sm" variant="outline" pressed={pressed} onPressedChange={onChange} className={cn("gap-1.5", pressedClass)}>
      {label}
      <Kbd>{k}</Kbd>
    </Toggle>
  )
}
