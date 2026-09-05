import { Button as ButtonPrimitive } from "@base-ui/react/button"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "cn"

// Two kinds of control: a filled black button for the one thing to do on a
// page, and words for everything else. Size comes before variant so the
// text variants can drop the box.
const buttonVariants = cva(
  "group/button inline-flex shrink-0 items-center justify-center gap-1.5 whitespace-nowrap text-sm font-medium transition-colors outline-none select-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-foreground disabled:pointer-events-none disabled:opacity-50 aria-invalid:text-destructive [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      size: {
        default: "h-9 px-3.5",
        xs: "h-7 px-2 text-xs [&_svg:not([class*='size-'])]:size-3",
        sm: "h-8 px-3 text-[13px] [&_svg:not([class*='size-'])]:size-3.5",
        lg: "h-10 px-4",
        icon: "size-8",
        "icon-xs": "size-6 [&_svg:not([class*='size-'])]:size-3",
        "icon-sm": "size-7",
        "icon-lg": "size-9",
      },
      variant: {
        default: "bg-foreground text-background hover:bg-[#333333]",
        secondary: "border border-border bg-transparent text-foreground hover:bg-muted",
        outline:
          "h-auto px-0 font-normal text-foreground underline decoration-[#b8b8b4] underline-offset-4 hover:decoration-foreground aria-expanded:decoration-foreground",
        ghost: "text-muted-foreground hover:text-foreground aria-expanded:text-foreground",
        destructive:
          "h-auto px-0 font-normal text-destructive underline decoration-[#e3b4b4] underline-offset-4 hover:decoration-destructive",
        link: "h-auto px-0 font-normal underline decoration-[#b8b8b4] underline-offset-4 hover:decoration-foreground",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

function Button({
  className,
  variant = "default",
  size = "default",
  ...props
}: ButtonPrimitive.Props & VariantProps<typeof buttonVariants>) {
  return (
    <ButtonPrimitive
      data-slot="button"
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  )
}

export { Button, buttonVariants }
