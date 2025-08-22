---
sidebar_position: 1
---

# JWT Authentication

JSON Web Tokens (JWT) are used throughout the Control Plane for authentication and authorization.

## Overview

JWT is an open standard (RFC 7519) for securely transmitting information between parties as a JSON object. The Control Plane uses JWTs for:

- User authentication
- Service-to-service communication
- Role-based access control

## JWT Integration

The Control Plane uses the `github.com/gofiber/contrib/jwt` middleware for JWT authentication:

```go
func setupAuth(app *fiber.App) {
    app.Use(jwt.New(jwt.Config{
        SigningKey:    []byte(os.Getenv("JWT_SECRET")),
        SigningMethod: "HS256",
        ErrorHandler: func(c *fiber.Ctx, err error) error {
            return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
                "error": "Unauthorized",
                "message": err.Error(),
            })
        },
    }))
}
```

## JWT Structure

JWTs used in the Control Plane follow a standard structure:

```json
{
  "header": {
    "alg": "RS256",
    "typ": "JWT",
    "kid": "2022-key-1"
  },
  "payload": {
    "sub": "user-123",
    "name": "John Doe",
    "roles": ["admin", "editor"],
    "groups": ["group-123", "group-456"],
    "exp": 1640995200,
    "iat": 1640908800,
    "iss": "https://auth.telekom.de"
  },
  "signature": "..."
}
```

## RBAC with JWT

The Control Plane implements Role-Based Access Control (RBAC) using JWT claims:

```go
func checkAccess(ctx *fiber.Ctx, requiredRole string) bool {
    user := ctx.Locals("user").(*jwt.Token)
    claims := user.Claims.(jwt.MapClaims)
    
    // Extract roles from the token
    rolesData := claims["roles"]
    if rolesData == nil {
        return false
    }
    
    roles := make([]string, 0)
    if rolesArray, ok := rolesData.([]interface{}); ok {
        for _, role := range rolesArray {
            if roleStr, ok := role.(string); ok {
                roles = append(roles, roleStr)
            }
        }
    }
    
    // Check if the user has the required role
    for _, role := range roles {
        if role == requiredRole {
            return true
        }
    }
    
    return false
}
```

## Token Validation

The Control Plane validates JWTs for:

- Signature integrity
- Expiration time
- Issuer (iss)
- Audience (aud)

```go
func validateToken(tokenString string) (*jwt.Token, error) {
    token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
        // Validate signing method
        if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
        }
        
        // Return the secret key for validation
        return []byte(os.Getenv("JWT_SECRET")), nil
    })
    
    if err != nil {
        return nil, err
    }
    
    // Check if token is valid
    if !token.Valid {
        return nil, errors.New("invalid token")
    }
    
    return token, nil
}
```

## Token Generation

Services in the Control Plane generate JWTs for clients:

```go
func generateToken(userID string, roles []string) (string, error) {
    // Create claims
    claims := jwt.MapClaims{
        "sub":   userID,
        "roles": roles,
        "exp":   time.Now().Add(time.Hour * 24).Unix(),
        "iat":   time.Now().Unix(),
        "iss":   "controlplane",
    }
    
    // Create token with claims
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    
    // Generate signed token
    signedToken, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
    if err != nil {
        return "", err
    }
    
    return signedToken, nil
}
```