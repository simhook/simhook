import { Suspense } from "react";
import { SignedInRedirect } from "@/components/signed-in-redirect";
import { Bar, Footer, PUBLIC_LINKS, Shell } from "@/components/site-chrome";

/** Sign-in and friends are pages of the site: the same bar and footer, a form-width column inside. */
export default function AuthLayout({ children }: LayoutProps<"/">) {
  return (
    <Shell>
      <Suspense>
        <SignedInRedirect />
      </Suspense>
      <Bar links={PUBLIC_LINKS} right={<span className="text-foreground">Sign in</span>} />
      <main className="mx-auto w-full max-w-[440px] flex-1 pt-12">{children}</main>
      <Footer />
    </Shell>
  );
}
