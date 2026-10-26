import React, {useEffect, useRef, useState, type ReactNode} from 'react';
import clsx from 'clsx';
import {translate} from '@docusaurus/Translate';
import {useAnchorTargetClassName} from '@docusaurus/theme-common';
import Link from '@docusaurus/Link';
import useBrokenLinks from '@docusaurus/useBrokenLinks';
import type {Props} from '@theme/Heading';
import styles from './styles.module.css';

// Swizzled (ejected) copy of @docusaurus/theme-classic's Heading. The
// one change: clicking a heading's `#` link copies the heading's full
// URL to the clipboard and says so for a moment, in addition to the
// default behaviour of putting the hash in the address bar. The link
// is also kept visible on touch screens, where there is no hover to
// reveal it.

const COPIED_FOR_MS = 1500;

function CopyHashLink({id, title}: {id: string; title: string}): ReactNode {
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => () => clearTimeout(timer.current), []);

  const onClick = () => {
    if (typeof window === 'undefined' || !navigator.clipboard) {
      return;
    }
    const url = `${window.location.origin}${window.location.pathname}#${id}`;
    navigator.clipboard.writeText(url).then(
      () => {
        setCopied(true);
        clearTimeout(timer.current);
        timer.current = setTimeout(() => setCopied(false), COPIED_FOR_MS);
      },
      () => {
        // Clipboard access denied: the default navigation still put the
        // hash in the address bar, so there is nothing more to do.
      },
    );
  };

  return (
    <Link
      className={clsx('hash-link', styles.hashLink, copied && styles.copied)}
      to={`#${id}`}
      onClick={onClick}
      aria-label={title}
      title={title}
      translate="no">
      &#8203;
      {copied && (
        <span className={styles.toast} role="status">
          {translate({
            id: 'theme.common.headingLinkCopied',
            message: 'Link copied',
            description: 'Shown after a heading link was copied to the clipboard',
          })}
        </span>
      )}
    </Link>
  );
}

export default function Heading({as: As, id, ...props}: Props): ReactNode {
  const brokenLinks = useBrokenLinks();
  const anchorTargetClassName = useAnchorTargetClassName(id);
  // H1 headings do not need an id because they don't appear in the TOC.
  if (As === 'h1' || !id) {
    return <As {...props} id={undefined} />;
  }
  brokenLinks.collectAnchor(id);
  const anchorTitle = translate(
    {
      id: 'theme.common.headingLinkTitle',
      message: 'Direct link to {heading}',
      description: 'Title for link to heading',
    },
    {
      heading: typeof props.children === 'string' ? props.children : id,
    },
  );
  return (
    <As
      {...props}
      className={clsx('anchor', anchorTargetClassName, props.className)}
      id={id}>
      {props.children}
      <CopyHashLink id={id} title={anchorTitle} />
    </As>
  );
}
