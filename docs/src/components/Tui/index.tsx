import type { ReactNode } from 'react';
import Heading from '@theme/Heading';
import TerminalWindow from '@site/src/components/TerminalWindow';
import styles from './styles.module.css';

// `onctl tui`, shown rather than described: the screenshot is the real
// view (cmd/tuirender.go) rendered against sample Hetzner data at
// 110x18, so the hotkeys, columns and colours are what the command
// actually draws.
export default function Tui(): ReactNode {
  return (
    <section className={styles.section}>
      <div className="container">
        <div className={styles.text}>
          <Heading as="h2" id="tui">Or watch them all at once</Heading>
          <p>
            <code>onctl tui</code> is the VM list as a live, k9s-style view. <code>s</code> opens
            a shell on the VM under the cursor, <code>c</code> creates one, <code>p</code> and{' '}
            <code>r</code> pause and resume, <code>ctrl-d</code> destroys (after asking),{' '}
            <code>:images</code> and <code>:sizes</code> show what the provider offers, and{' '}
            <code>?</code> lists every key.
          </p>
        </div>
        <div className={styles.screenshot}>
          <TerminalWindow title="onctl tui">
            <img
              src="/img/tui.png"
              alt="onctl tui: a header with the provider, VM counts and hotkeys, and a table of VMs with their state, public IP, type, image, location, age and hourly cost"
              width={2068}
              height={774}
              loading="lazy"
            />
          </TerminalWindow>
        </div>
      </div>
    </section>
  );
}
