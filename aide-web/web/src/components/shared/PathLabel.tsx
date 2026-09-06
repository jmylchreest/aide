import { useProjectRoot } from "@/context/ProjectRootContext";
import { relativeToRoot } from "@/lib/paths";
import { cn } from "@/lib/utils";

/** Keep the filename visible when the available column width is exhausted. */
export function PathLabel({
  path,
  displayPath,
  suffix = "",
  className,
}: {
  path: string;
  displayPath?: string;
  suffix?: string;
  className?: string;
}) {
  const root = useProjectRoot();
  return (
    <span
      dir="rtl"
      title={path + suffix}
      className={cn("block min-w-0 truncate text-left font-mono", className)}
    >
      <bdi dir="ltr">
        {displayPath || relativeToRoot(path, root)}
        {suffix}
      </bdi>
    </span>
  );
}
