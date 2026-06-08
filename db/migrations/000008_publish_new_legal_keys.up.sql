UPDATE documents
   SET is_publicly_visible = TRUE
 WHERE key IN ('gdpr-rights', 'terms-of-service-passenger', 'terms-of-service-driver');
