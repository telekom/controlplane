import type {ReactNode} from 'react';
import Content from '@theme-original/DocItem/Content';
import type ContentType from '@theme/DocItem/Content';
import type {WrapperProps} from '@docusaurus/types';
import {
  useActivePluginAndVersion,
  useDoc,
} from '@docusaurus/plugin-content-docs/client';
import Admonition from '@theme/Admonition';
import Link from '@docusaurus/Link';

type Props = WrapperProps<typeof ContentType>;

type DecisionFrontMatter = {
  status?: string;
  origin?: string;
  date?: string | Date;
  evidence?: string[];
  'superseded-by'?: number | string;
};

const REPOSITORY = 'https://github.com/telekom/controlplane';

const BANNERS: Record<
  string,
  {type: 'tip' | 'note' | 'warning'; title: string; text: string}
> = {
  proposed: {
    type: 'warning',
    title: 'Proposed',
    text: 'This record is a draft awaiting review. It may be incomplete or wrong; do not rely on it yet.',
  },
  accepted: {
    type: 'tip',
    title: 'Accepted',
    text: 'The team stands behind this decision.',
  },
  rejected: {
    type: 'note',
    title: 'Rejected',
    text: 'This option was considered and turned down. The record explains why.',
  },
  deprecated: {
    type: 'warning',
    title: 'Deprecated',
    text: 'This decision no longer applies.',
  },
  superseded: {
    type: 'warning',
    title: 'Superseded',
    text: 'A newer record replaces this decision.',
  },
};

function sourceLink(source: string): string {
  if (/^https?:\/\//.test(source)) {
    return source;
  }
  if (/^[0-9a-f]{7,40}$/.test(source)) {
    return `${REPOSITORY}/commit/${source}`;
  }
  return `${REPOSITORY}/blob/main/${source}`;
}

function isoDate(value: string | Date | undefined): string | undefined {
  if (value instanceof Date) {
    return value.toISOString().slice(0, 10);
  }
  return value ? String(value).slice(0, 10) : undefined;
}

function DecisionBanner({frontMatter}: {frontMatter: DecisionFrontMatter}): ReactNode {
  const docs = useActivePluginAndVersion()?.activeVersion?.docs ?? [];
  const banner = BANNERS[frontMatter.status ?? ''];
  if (!banner) {
    return null;
  }
  const successor = frontMatter['superseded-by'];
  const successorRef =
    successor === undefined
      ? undefined
      : String(successor).trim().replace(/^ADR-/i, '').padStart(4, '0');
  const successorDoc = successorRef
    ? docs.find((doc) => doc.id.startsWith(`decisions/${successorRef}-`))
    : undefined;
  const date = isoDate(frontMatter.date);
  const sources = frontMatter.evidence ?? [];

  return (
    <Admonition type={banner.type} title={date ? `${banner.title} · ${date}` : banner.title}>
      <p>
        {banner.text}
        {successorRef && (
          <>
            {' '}
            See{' '}
            {successorDoc ? (
              <Link to={successorDoc.path}>ADR-{successorRef}</Link>
            ) : (
              `ADR-${successorRef}`
            )}
            .
          </>
        )}
      </p>
      {frontMatter.origin === 'backfilled' && (
        <p>Reconstructed after the decision was made. Check it against its sources.</p>
      )}
      {sources.length > 0 && (
        <ul>
          {sources.map((source) => (
            <li key={source}>
              <Link to={sourceLink(source)}>{source}</Link>
            </li>
          ))}
        </ul>
      )}
    </Admonition>
  );
}

export default function ContentWrapper(props: Props): ReactNode {
  const {metadata, frontMatter} = useDoc();
  const decision = frontMatter as DecisionFrontMatter;
  const isDecision =
    metadata.sourceDirName === 'decisions' && typeof decision.status === 'string';
  return (
    <>
      {isDecision && <DecisionBanner frontMatter={decision} />}
      <Content {...props} />
    </>
  );
}
