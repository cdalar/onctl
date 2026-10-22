import type { ReactNode } from 'react';
import clsx from 'clsx';
import styles from './styles.module.css';

export type TerminalLine =
  | { type: 'prompt'; text: string }
  | { type: 'output'; text: string; dim?: boolean };

// The one structural device this page reuses: a real terminal window, used
// wherever the content actually is a terminal session (the hero's install
// + first boot, and the full create/ssh/ls/destroy lifecycle further
// down, and the `onctl tui` screenshot, which is a terminal too) -- not a
// decorative card shape applied to unrelated content. Pass `lines` for a
// typed-out session, or children (an image) to frame as-is.
export default function TerminalWindow({
  title,
  lines,
  showCursor = false,
  children,
}: {
  title: string;
  lines?: TerminalLine[];
  showCursor?: boolean;
  children?: ReactNode;
}): ReactNode {
  return (
    <div className={styles.window}>
      <div className={styles.titlebar}>
        <span className={styles.dots} aria-hidden="true">
          <span className={styles.dot} />
          <span className={styles.dot} />
          <span className={styles.dot} />
        </span>
        <span className={styles.title}>{title}</span>
      </div>
      {children ? (
        <div className={styles.screen}>{children}</div>
      ) : (
        <pre className={styles.body}>
          {(lines ?? []).map((line, i) => (
            <div
              key={i}
              className={clsx(styles.line, line.type === 'output' && line.dim && styles.dim)}
            >
              {line.type === 'prompt' ? (
                <>
                  <span className={styles.prompt}>❯</span> {line.text}
                </>
              ) : (
                line.text
              )}
            </div>
          ))}
          {showCursor && <span className={styles.cursor} aria-hidden="true" />}
        </pre>
      )}
    </div>
  );
}
