import React from 'react';
import Link from '@docusaurus/Link';

type ButtonLinkProps = {
  to: string;
  children: React.ReactNode;
  fullWidth?: boolean;
  primary?: boolean;
};

/**
 * Button-styled link component for consistent call-to-action buttons
 */
export default function ButtonLink({
  to,
  children,
  fullWidth = false,
  primary = true,
}: ButtonLinkProps): JSX.Element {
  const buttonClasses = [
    'button',
    primary ? 'button--primary' : 'button--secondary',
    fullWidth && 'button--block',
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <Link className={buttonClasses} to={to}>
      {children}
    </Link>
  );
}