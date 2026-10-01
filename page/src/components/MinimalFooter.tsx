import { Heart, Github, ExternalLink } from 'lucide-react';

export const MinimalFooter = () => {
  return (
    <footer className="mt-16 border-t border-[#222222] py-8 text-xs font-mono text-[#8d8983]">
      <div className="max-w-4xl mx-auto px-4 sm:px-6 flex flex-col sm:flex-row items-center justify-between gap-4">
        
        <div className="flex items-center gap-2 text-center sm:text-left">
          <span className="w-2 h-2 rounded-full bg-[#e17a56]"></span>
          <span className="text-[#ece8e2] font-bold">golunch</span>
          <span>·</span>
          <span>MIT License</span>
          <span>·</span>
          <span>Go 1.27+</span>
        </div>

        <div className="flex items-center gap-4 text-xs">
          <a
            href="https://github.com/raaaas/golunch"
            target="_blank"
            rel="noopener noreferrer"
            className="hover:text-[#ece8e2] transition-colors flex items-center gap-1"
          >
            <Github className="w-3.5 h-3.5" />
            <span>github</span>
          </a>

          <a
            href="https://github.com/sponsors/raaaas"
            target="_blank"
            rel="noopener noreferrer"
            className="hover:text-[#ff7a66] transition-colors flex items-center gap-1 text-[#e17a56]"
          >
            <Heart className="w-3.5 h-3.5 fill-[#e17a56]/20" />
            <span>sponsor</span>
          </a>

          <a
            href="https://pkg.go.dev/github.com/raaaas/golunch"
            target="_blank"
            rel="noopener noreferrer"
            className="hover:text-[#ece8e2] transition-colors flex items-center gap-1"
          >
            <span>pkg.go.dev</span>
            <ExternalLink className="w-3 h-3" />
          </a>
        </div>

      </div>
    </footer>
  );
};
