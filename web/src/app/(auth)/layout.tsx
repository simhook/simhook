import Link from "next/link";

export default function AuthLayout({ children }: LayoutProps<"/">) {
  return (
    <div className="mx-auto w-full max-w-[400px] px-6 pb-16">
      <div className="border-b py-4">
        <Link href="https://simhook.dev" className="font-mono text-[15px] font-medium">
          simhook
        </Link>
      </div>
      <div className="pt-10">{children}</div>
    </div>
  );
}
