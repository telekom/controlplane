import React, { ReactNode } from 'react';

type InfoSectionProps = {
  title?: string;
  children: ReactNode;
  type?: 'info' | 'note' | 'tip' | 'warning' | 'danger';
};

/**
 * Information section component for contextual information blocks
 * Uses Docusaurus admonition styling under the hood
 */
export default function InfoSection({
  title,
  children,
  type = 'info',
}: InfoSectionProps): JSX.Element {
  return (
    <div className={`admonition admonition-${type} alert alert--${type}`}>
      <div className="admonition-heading">
        {title && <h5>{title}</h5>}
      </div>
      <div className="admonition-content">{children}</div>
    </div>
  );
}