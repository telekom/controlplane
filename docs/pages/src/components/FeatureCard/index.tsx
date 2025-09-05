import React from 'react';
import Link from '@docusaurus/Link';

type FeatureCardProps = {
  title: string;
  description: string | JSX.Element;
  icon?: string;
  linkText?: string;
  linkUrl?: string;
};

/**
 * Reusable Feature Card component for consistent feature presentation
 */
export default function FeatureCard({
  title,
  description,
  icon,
  linkText,
  linkUrl,
}: FeatureCardProps): JSX.Element {
  return (
    <div className="card margin-bottom--lg">
      <div className="card__header">
        <h3>{icon && <span>{icon} </span>}{title}</h3>
      </div>
      <div className="card__body">
        {typeof description === 'string' ? <p>{description}</p> : description}
      </div>
      {linkText && linkUrl && (
        <div className="card__footer">
          <Link className="button button--primary button--block" to={linkUrl}>
            {linkText}
          </Link>
        </div>
      )}
    </div>
  );
}