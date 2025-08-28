import React from 'react';

type PageHeaderProps = {
  title: string;
  description?: string;
  logo?: {
    src: string;
    alt: string;
    width?: number;
  };
};

/**
 * Consistent page header component with optional logo and description
 */
export default function PageHeader({
  title,
  description,
  logo,
}: PageHeaderProps): JSX.Element {
  return (
    <div style={{textAlign: 'center', marginBottom: '2rem'}}>
      {logo && (
        <img 
          src={logo.src} 
          alt={logo.alt} 
          width={logo.width || 200}
          style={{marginBottom: '1rem'}}
        />
      )}
      <h1>{title}</h1>
      {description && <p>{description}</p>}
    </div>
  );
}