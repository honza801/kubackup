package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
)

func Encrypt(reader io.Reader, writer io.Writer, key []byte) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}

	// Create a random IV (initialization vector)
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return err
	}

	// Write the IV to the output
	_, err = writer.Write(iv)
	if err != nil {
		return err
	}

	// Create a CTR cipher mode
	stream := cipher.NewCTR(block, iv)

	streamReader := cipher.StreamReader{S: stream, R: reader}
	// Encrypt and write the input to the output
	if _, err := io.Copy(writer, streamReader); err != nil {
		return err
	}

	return nil
}

func Decrypt(reader io.Reader, writer io.Writer, key []byte) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}

	// Read the IV from the input
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(reader, iv); err != nil {
		return err
	}

	// Create a CTR cipher mode with the extracted IV
	stream := cipher.NewCTR(block, iv)

	// Decrypt and write the input to the output
	streamReader := cipher.StreamReader{S: stream, R: reader}
	if _, err := io.Copy(writer, streamReader); err != nil {
		return err
	}

	return nil
}
