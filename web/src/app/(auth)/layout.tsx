import { Bar, Footer, PUBLIC_LINKS, Shell } from "@/components/site-chrome";

/** Sign-in and friends are pages of the site: the same bar and footer, a form-width column inside. */
export default function AuthLayout({ children }: LayoutProps<"/">) {
  return (
    <Shell>
      <Bar links={PUBLIC_LINKS} right={<span className="text-foreground">Sign in</span>} />
      <main className="mx-auto w-full max-w-[400px] flex-1 pt-12">{children}</main>
      <Footer />
    </Shell>
  );
}
