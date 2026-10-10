// Single source of truth for the Console's UI primitives. Import shared
// controls from here so every surface renders the same component: the
// motion primitives (Switch, AnimatedBadge) are the standard building blocks
// that status chips and toggles across the app are derived from.

export { AnimatedBadge } from "@/components/motion/animated-badge";
export type {
  AnimatedBadgeProps,
  AnimatedBadgeSize,
  AnimatedBadgeStatus,
} from "@/components/motion/animated-badge";

export { Switch } from "@/components/motion/switch";
export type { SwitchProps } from "@/components/motion/switch";

export { Badge, HttpStatusBadge, StatusBadge } from "@/components/ui/badge";
export { Button, buttonVariants } from "@/components/ui/button";
export {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
export { Input } from "@/components/ui/input";
export { Label } from "@/components/ui/label";
export { PixelSkeleton } from "@/components/ui/pixel-skeleton";
export { Skeleton } from "@/components/ui/skeleton";
export {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
export { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
export { Textarea } from "@/components/ui/textarea";
